package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"modern-dhcp/internal/failover"
)

const (
	fencingEpochHeader = "X-Fencing-Epoch"
	fencingEpochQuery  = "fencingEpoch"
)

func (s *HTTPServer) primaryWriteGuardMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if s == nil || c == nil {
				return next(c)
			}
			if !isWriteMethod(c.Request().Method) {
				return next(c)
			}
			if isPrimaryWriteGuardExemptPath(c.Path()) {
				return next(c)
			}
			snapshot, ok := s.failoverSnapshot()
			if !ok {
				return next(c)
			}
			if snapshot.Role == failover.RolePrimary || snapshot.Role == failover.RoleUnknown {
				expectedEpoch := strings.TrimSpace(snapshot.FencingEpoch)
				if expectedEpoch == "" {
					return next(c)
				}
				providedEpoch := strings.TrimSpace(c.Request().Header.Get(fencingEpochHeader))
				if providedEpoch == "" {
					providedEpoch = strings.TrimSpace(c.QueryParam(fencingEpochQuery))
				}
				if providedEpoch != expectedEpoch {
					s.auditWriteGuardReject(c, "stale_fencing_epoch", map[string]any{
						"path":          c.Path(),
						"method":        c.Request().Method,
						"role":          string(snapshot.Role),
						"state":         string(snapshot.State),
						"expectedEpoch": expectedEpoch,
						"providedEpoch": providedEpoch,
					})
					return echo.NewHTTPError(http.StatusConflict, map[string]any{
						"code":          "stale_fencing_epoch",
						"message":       fmt.Sprintf("fencing epoch mismatch, include %s header", fencingEpochHeader),
						"expectedEpoch": expectedEpoch,
						"providedEpoch": providedEpoch,
					})
				}
				return next(c)
			}
			s.auditWriteGuardReject(c, "standby_write_blocked", map[string]any{
				"path":   c.Path(),
				"method": c.Request().Method,
				"role":   string(snapshot.Role),
				"state":  string(snapshot.State),
			})
			return echo.NewHTTPError(http.StatusLocked, map[string]any{
				"code":    "standby_write_blocked",
				"message": "write operations are only allowed on primary node",
				"role":    string(snapshot.Role),
				"state":   string(snapshot.State),
			})
		}
	}
}

func (s *HTTPServer) failoverSnapshot() (failover.StatusSnapshot, bool) {
	if s == nil {
		return failover.StatusSnapshot{}, false
	}
	if s.options.FailoverController != nil {
		return s.options.FailoverController.Snapshot(), true
	}
	if s.options.Coordinator != nil {
		return s.options.Coordinator.Snapshot(), true
	}
	return failover.StatusSnapshot{}, false
}

func isWriteMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isPrimaryWriteGuardExemptPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "/auth/")
}

func (s *HTTPServer) auditWriteGuardReject(c echo.Context, code string, payload map[string]any) {
	if s == nil || c == nil {
		return
	}
	ctx := c.Request().Context()
	s.recordAudit(ctx, s.auditTenantFromContext(c), s.actorFromContext(c), "ha.write_guard.reject", payload, withResource("ha"))
}
