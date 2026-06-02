package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"modern-dhcp/internal/failover"
)

type writeGuardCoordinatorStub struct {
	snapshot failover.StatusSnapshot
}

func (w writeGuardCoordinatorStub) Snapshot() failover.StatusSnapshot {
	return w.snapshot
}

func TestPrimaryWriteGuardBlocksStandbyRequests(t *testing.T) {
	s := &HTTPServer{
		logger:  zap.NewNop(),
		options: Options{Coordinator: writeGuardCoordinatorStub{snapshot: failover.StatusSnapshot{Role: failover.RoleStandby, State: failover.StateNormal, FencingEpoch: "10"}}},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/nodes", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetPath("/cluster/nodes")

	h := s.primaryWriteGuardMiddleware()(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	err := h(ctx)
	if err == nil {
		t.Fatalf("expected standby write to be blocked")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected echo.HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusLocked {
		t.Fatalf("expected status %d, got %d", http.StatusLocked, httpErr.Code)
	}
}

func TestPrimaryWriteGuardBlocksStaleFencingEpoch(t *testing.T) {
	s := &HTTPServer{
		logger:  zap.NewNop(),
		options: Options{Coordinator: writeGuardCoordinatorStub{snapshot: failover.StatusSnapshot{Role: failover.RolePrimary, State: failover.StateNormal, FencingEpoch: "42"}}},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/nodes", nil)
	req.Header.Set(fencingEpochHeader, "41")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetPath("/cluster/nodes")

	h := s.primaryWriteGuardMiddleware()(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	err := h(ctx)
	if err == nil {
		t.Fatalf("expected stale fencing epoch to be blocked")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected echo.HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, httpErr.Code)
	}
}

func TestPrimaryWriteGuardAllowsValidFencingEpoch(t *testing.T) {
	s := &HTTPServer{
		logger:  zap.NewNop(),
		options: Options{Coordinator: writeGuardCoordinatorStub{snapshot: failover.StatusSnapshot{Role: failover.RolePrimary, State: failover.StateNormal, FencingEpoch: "99"}}},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/nodes", nil)
	req.Header.Set(fencingEpochHeader, "99")
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetPath("/cluster/nodes")

	h := s.primaryWriteGuardMiddleware()(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	err := h(ctx)
	if err != nil {
		t.Fatalf("expected valid fencing epoch request to pass, got %v", err)
	}
}
