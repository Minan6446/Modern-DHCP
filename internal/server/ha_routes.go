package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/ha"
)

// registerHARoutes wires HA control surface endpoints under /api/{version}/ha.
func (s *HTTPServer) registerHARoutes() {
	if s == nil || s.echo == nil {
		return
	}
	if s.options.FailoverController == nil {
		return
	}
	if s.ensureHAService() == nil {
		return
	}
	versions := s.apiVersions
	if len(versions) == 0 {
		versions = []string{"v1"}
	}
	for _, version := range versions {
		base := fmt.Sprintf("/api/%s/ha", version)
		group := s.echo.Group(base)
		s.applyAPIMiddleware(group)
		group.GET("/nodes", s.handleHANodes(), RequireCapability(CapabilityHARead))
		group.GET("/runbooks", s.handleHARunbooks(), RequireCapability(CapabilityHARead))
		group.POST("/failover", s.handleHAFailover(), RequireCapability(CapabilityHAManage))
		group.PUT("/load-balancer", s.handleHALoadPolicy(), RequireCapability(CapabilityHAManage))
	}
}

func (s *HTTPServer) applyAPIMiddleware(group *echo.Group) {
	if s == nil || group == nil {
		return
	}
	if s.options.RequireAuth && s.auth != nil {
		group.Use(s.auth.Middleware())
		group.Use(s.requireSuperAdminPasswordReset())
	}
	group.Use(s.resolveCapabilitiesMiddleware())
	if s.rateLimit != nil {
		group.Use(s.rateLimit.Middleware())
	}
	if s.quota != nil {
		group.Use(s.quota.Middleware())
	}
}

func (s *HTTPServer) handleHANodes() echo.HandlerFunc {
	type response struct {
		GeneratedAt time.Time               `json:"generatedAt"`
		Snapshot    failover.StatusSnapshot `json:"snapshot"`
		Nodes       []failover.NodeStatus   `json:"nodes"`
	}
	return func(c echo.Context) error {
		svc, err := s.requireHAService()
		if err != nil {
			return err
		}
		nodes := svc.Nodes(c.Request().Context())
		if nodes == nil {
			nodes = []failover.NodeStatus{}
		}
		payload := response{GeneratedAt: time.Now().UTC(), Snapshot: svc.Snapshot(), Nodes: nodes}
		return c.JSON(http.StatusOK, payload)
	}
}

func (s *HTTPServer) handleHARunbooks() echo.HandlerFunc {
	type response struct {
		GeneratedAt time.Time    `json:"generatedAt"`
		Runbooks    []ha.Runbook `json:"runbooks"`
	}
	return func(c echo.Context) error {
		svc, err := s.requireHAService()
		if err != nil {
			return err
		}
		books := svc.Runbooks(c.Request().Context())
		if books == nil {
			books = []ha.Runbook{}
		}
		return c.JSON(http.StatusOK, response{GeneratedAt: time.Now().UTC(), Runbooks: books})
	}
}

func (s *HTTPServer) handleHAFailover() echo.HandlerFunc {
	type request struct {
		TargetRole string `json:"targetRole"`
		Reason     string `json:"reason"`
		DryRun     bool   `json:"dryRun"`
		Force      bool   `json:"force"`
	}
	return func(c echo.Context) error {
		svc, err := s.requireHAService()
		if err != nil {
			return err
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		req := ha.FailoverRequest{
			TargetRole: payload.TargetRole,
			Reason:     payload.Reason,
			DryRun:     payload.DryRun,
			Force:      payload.Force,
		}
		ctx := c.Request().Context()
		if err := svc.RequestFailover(ctx, req); err != nil {
			return translateFailoverError(err)
		}
		actor := s.actorFromContext(c)
		auditPayload := map[string]any{
			"targetRole": req.TargetRole,
			"reason":     req.Reason,
			"dryRun":     req.DryRun,
			"force":      req.Force,
		}
		s.recordAudit(ctx, s.auditTenantFromContext(c), actor, "ha.failover", auditPayload, withResource("ha"))
		status := http.StatusAccepted
		if payload.DryRun {
			status = http.StatusOK
		}
		response := map[string]any{
			"status":     "ok",
			"targetRole": req.TargetRole,
			"dryRun":     payload.DryRun,
		}
		return c.JSON(status, response)
	}
}

func (s *HTTPServer) handleHALoadPolicy() echo.HandlerFunc {
	type request struct {
		Strategy      string         `json:"strategy"`
		Weights       map[string]int `json:"weights"`
		StickySeconds int64          `json:"stickySeconds"`
	}
	return func(c echo.Context) error {
		svc, err := s.requireHAService()
		if err != nil {
			return err
		}
		var payload request
		if err := c.Bind(&payload); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if strings.TrimSpace(payload.Strategy) == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "strategy is required")
		}
		if payload.StickySeconds < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "stickySeconds must be positive")
		}
		req := ha.LoadPolicy{Strategy: payload.Strategy, Weights: payload.Weights, StickySeconds: payload.StickySeconds}
		if err := svc.UpdateLoadPolicy(c.Request().Context(), req); err != nil {
			return translateLoadPolicyError(err)
		}
		actor := s.actorFromContext(c)
		s.recordAudit(c.Request().Context(), s.auditTenantFromContext(c), actor, "ha.load_balancer", map[string]any{
			"strategy": req.Strategy,
			"weights":  req.Weights,
			"sticky":   req.StickySeconds,
		}, withResource("ha"))
		return c.JSON(http.StatusOK, map[string]any{"status": "updated", "strategy": req.Strategy})
	}
}

func (s *HTTPServer) requireHAService() (*ha.Service, error) {
	svc := s.ensureHAService()
	if svc == nil {
		return nil, echo.NewHTTPError(http.StatusServiceUnavailable, "ha service disabled")
	}
	return svc, nil
}

func (s *HTTPServer) ensureHAService() *ha.Service {
	if s == nil {
		return nil
	}
	s.haOnce.Do(func() {
		if s.haSvc != nil || s.options.FailoverController == nil {
			return
		}
		s.haSvc = ha.NewService(ha.Options{Controller: s.options.FailoverController, RunbookPaths: s.options.HARunbooks, Logger: s.logger})
	})
	return s.haSvc
}

func translateFailoverError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return echo.NewHTTPError(http.StatusRequestTimeout, err.Error())
	case errors.Is(err, ha.ErrControllerUnavailable), errors.Is(err, failover.ErrManualFailoverDisabled):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, failover.ErrManualFailoverPeerHealthy):
		return echo.NewHTTPError(http.StatusPreconditionFailed, err.Error())
	case errors.Is(err, failover.ErrManualFailoverInvalid):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}

func translateLoadPolicyError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ha.ErrControllerUnavailable):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ha.ErrInvalidLoadPolicy):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, failover.ErrIngressControllerUnavailable):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}
