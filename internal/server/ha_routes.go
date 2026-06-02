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
		group.GET("/status", s.handleHAStatus(), RequireCapability(CapabilityHARead))
		group.GET("/nodes", s.handleHANodes(), RequireCapability(CapabilityHARead))
		group.GET("/runbooks", s.handleHARunbooks(), RequireCapability(CapabilityHARead))
		group.GET("/membership-events", s.handleHAMembershipEvents(), RequireCapability(CapabilityHARead))
		group.GET("/drills/history", s.handleHADrillHistory(), RequireCapability(CapabilityHARead))
		group.POST("/failover", s.handleHAFailover(), RequireCapability(CapabilityHAManage))
		group.POST("/drills/dry-run", s.handleHADrillDryRun(), RequireCapability(CapabilityHAManage))
		group.POST("/members", s.handleHAMemberUpsert(), RequireCapability(CapabilityHAManage))
		group.DELETE("/members/:nodeId", s.handleHAMemberRemove(), RequireCapability(CapabilityHAManage))
		group.PUT("/load-balancer", s.handleHALoadPolicy(), RequireCapability(CapabilityHAManage))
		group.POST("/allow-failback", s.handleHAAllowFailback(), RequireCapability(CapabilityHAManage))
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
	group.Use(s.csrfMiddleware())
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

func (s *HTTPServer) handleHAStatus() echo.HandlerFunc {
	type response struct {
		GeneratedAt time.Time               `json:"generatedAt"`
		Snapshot    failover.StatusSnapshot `json:"snapshot"`
	}
	return func(c echo.Context) error {
		svc, err := s.requireHAService()
		if err != nil {
			return err
		}
		payload := response{GeneratedAt: time.Now().UTC(), Snapshot: svc.Snapshot()}
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

func (s *HTTPServer) handleHAMembershipEvents() echo.HandlerFunc {
	type response struct {
		GeneratedAt time.Time              `json:"generatedAt"`
		Items       []clusterFailoverEvent `json:"items"`
		Count       int                    `json:"count"`
	}
	return func(c echo.Context) error {
		items := s.currentMembershipEvents()
		return c.JSON(http.StatusOK, response{GeneratedAt: time.Now().UTC(), Items: items, Count: len(items)})
	}
}

func (s *HTTPServer) handleHAMemberUpsert() echo.HandlerFunc {
	type request struct {
		ID       string `json:"id"`
		Address  string `json:"address"`
		Role     string `json:"role"`
		Region   string `json:"region"`
		Zone     string `json:"zone"`
		Weight   int    `json:"weight"`
		Disabled bool   `json:"disabled"`
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
		id := strings.TrimSpace(payload.ID)
		if id == "" {
			id = strings.TrimSpace(payload.Address)
		}
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "id or address is required")
		}
		role := strings.ToLower(strings.TrimSpace(payload.Role))
		reg := failover.NodeRegistration{
			ID:       id,
			Address:  strings.TrimSpace(payload.Address),
			Region:   strings.TrimSpace(payload.Region),
			Zone:     strings.TrimSpace(payload.Zone),
			Weight:   payload.Weight,
			Disabled: payload.Disabled,
			Role:     failover.RoleStandby,
		}
		if role == "active" || role == "primary" {
			reg.Role = failover.RolePrimary
		}
		ctx := c.Request().Context()
		if err := svc.UpsertNode(ctx, reg); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.syncClusterNodeFromHAMember(ctx, reg)
		s.appendScaleEvent(clusterFailoverEvent{
			ID:          fmt.Sprintf("ha-members-upsert-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "动态成员写入",
			Detail:      fmt.Sprintf("HA成员 %s 已更新（role=%s, disabled=%t）", reg.ID, reg.Role, reg.Disabled),
			Status:      "success",
			Type:        "info",
			StatusLabel: "完成",
		})
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "ha.member.upsert", map[string]any{
			"id":       reg.ID,
			"address":  reg.Address,
			"role":     string(reg.Role),
			"disabled": reg.Disabled,
		}, withResource("ha"))
		nodes := svc.Nodes(ctx)
		return c.JSON(http.StatusOK, map[string]any{"status": "updated", "nodes": nodes})
	}
}

func (s *HTTPServer) handleHAMemberRemove() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireHAService()
		if err != nil {
			return err
		}
		nodeID := strings.TrimSpace(c.Param("nodeId"))
		if nodeID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "nodeId is required")
		}
		ctx := c.Request().Context()
		if err := svc.RemoveNode(ctx, nodeID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		s.removeClusterNodeForHAMember(ctx, nodeID)
		s.appendScaleEvent(clusterFailoverEvent{
			ID:          fmt.Sprintf("ha-members-remove-%d", time.Now().UnixNano()),
			Time:        time.Now().Format("2006-01-02 15:04:05"),
			Title:       "动态成员删除",
			Detail:      fmt.Sprintf("HA成员 %s 已删除", nodeID),
			Status:      "success",
			Type:        "warning",
			StatusLabel: "完成",
		})
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "ha.member.remove", map[string]any{
			"id": nodeID,
		}, withResource("ha"))
		return c.JSON(http.StatusOK, map[string]any{"status": "removed", "id": nodeID})
	}
}

func (s *HTTPServer) currentMembershipEvents() []clusterFailoverEvent {
	items := s.currentScaleEvents()
	result := make([]clusterFailoverEvent, 0, len(items))
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		if strings.Contains(title, "动态成员") || strings.Contains(strings.TrimSpace(item.ErrorCode), "MEMBERSHIP_") {
			result = append(result, item)
		}
	}
	return result
}

func (s *HTTPServer) handleHADrillHistory() echo.HandlerFunc {
	type response struct {
		GeneratedAt time.Time              `json:"generatedAt"`
		Items       []clusterFailoverEvent `json:"items"`
		Count       int                    `json:"count"`
	}
	return func(c echo.Context) error {
		items := s.currentDrillEvents()
		return c.JSON(http.StatusOK, response{GeneratedAt: time.Now().UTC(), Items: items, Count: len(items)})
	}
}

func (s *HTTPServer) handleHADrillDryRun() echo.HandlerFunc {
	type request struct {
		TargetRole string `json:"targetRole"`
		Reason     string `json:"reason"`
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
		reason := strings.TrimSpace(payload.Reason)
		if reason == "" {
			reason = "scheduled dry-run"
		}
		req := ha.FailoverRequest{
			TargetRole: strings.TrimSpace(payload.TargetRole),
			Reason:     reason,
			DryRun:     true,
			Force:      payload.Force,
		}
		ctx := c.Request().Context()
		if err := svc.RequestFailover(ctx, req); err != nil {
			return translateFailoverError(err)
		}
		now := time.Now().UTC()
		eventID := fmt.Sprintf("ha-drill-%d", now.UnixNano())
		event := clusterFailoverEvent{
			ID:          eventID,
			Time:        now.Format("2006-01-02 15:04:05"),
			Title:       "故障演练（Dry-Run）",
			Detail:      fmt.Sprintf("targetRole=%s reason=%s force=%t", req.TargetRole, req.Reason, req.Force),
			Status:      "success",
			Type:        "info",
			StatusLabel: "完成",
			TxID:        eventID,
			Phase:       "dry_run",
			ErrorCode:   "DRILL_DRY_RUN",
		}
		s.appendScaleEvent(event)
		s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "ha.drill.dry_run", map[string]any{
			"eventId":    eventID,
			"targetRole": req.TargetRole,
			"reason":     req.Reason,
			"force":      req.Force,
		}, withResource("ha"))
		return c.JSON(http.StatusAccepted, map[string]any{"status": "scheduled", "event": event})
	}
}

func (s *HTTPServer) currentDrillEvents() []clusterFailoverEvent {
	items := s.currentScaleEvents()
	result := make([]clusterFailoverEvent, 0, len(items))
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		if strings.Contains(title, "演练") || strings.HasPrefix(strings.TrimSpace(item.ErrorCode), "DRILL_") {
			result = append(result, item)
		}
	}
	return result
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

func (s *HTTPServer) handleHAAllowFailback() echo.HandlerFunc {
	return func(c echo.Context) error {
		svc, err := s.requireHAService()
		if err != nil {
			return err
		}
		ctx := c.Request().Context()
		if err := svc.AllowFailback(ctx); err != nil {
			return translateFailbackError(err)
		}
		actor := s.actorFromContext(c)
		s.recordAudit(ctx, s.auditTenantFromContext(c), actor, "ha.failback.allow", map[string]any{
			"manualFailback": true,
		}, withResource("ha"))
		return c.JSON(http.StatusAccepted, map[string]any{"status": "armed"})
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

func translateFailbackError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return echo.NewHTTPError(http.StatusRequestTimeout, err.Error())
	case errors.Is(err, ha.ErrControllerUnavailable):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}
