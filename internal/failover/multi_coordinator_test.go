package failover

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

type stubCoordinator struct {
	healthy atomic.Bool
	primary atomic.Bool
}

func (s *stubCoordinator) Start(ctx context.Context) {}
func (s *stubCoordinator) Stop()                     {}
func (s *stubCoordinator) Healthy() bool             { return s.healthy.Load() }
func (s *stubCoordinator) ShouldServePrimary() bool  { return s.primary.Load() }

func TestMultiCoordinatorMajority(t *testing.T) {
	c1 := &stubCoordinator{}
	c1.healthy.Store(true)
	c1.primary.Store(true)
	c2 := &stubCoordinator{}
	c2.healthy.Store(true)
	c2.primary.Store(false)
	c3 := &stubCoordinator{}
	c3.healthy.Store(false)
	c3.primary.Store(true)

	coord := newMultiCoordinator([]Coordinator{c1, c2, c3}, zap.NewNop())
	mc, ok := coord.(*multiCoordinator)
	if !ok {
		t.Fatalf("expected multiCoordinator, got %T", coord)
	}
	state := mc.CoordinatorQuorum()
	if state.Total != 3 || state.Healthy != 2 || state.PrimaryVotes != 1 {
		t.Fatalf("unexpected quorum state: %+v", state)
	}
	if !mc.Healthy() {
		t.Fatalf("expected coordinator to remain healthy with majority endpoints")
	}
	if mc.ShouldServePrimary() {
		t.Fatalf("expected coordinator to deny primary with insufficient votes")
	}

	c2.primary.Store(true)
	state = mc.CoordinatorQuorum()
	if !mc.ShouldServePrimary() {
		t.Fatalf("expected coordinator to grant primary once majority achieved")
	}
}

func TestHTTPCoordinatorTracksRemoteState(t *testing.T) {
	var allow atomic.Bool
	allow.Store(false)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"healthy":true,"allowPrimary":` + boolToString(allow.Load()) + `}`))
	}))
	defer server.Close()

	def := config.NamedCoordinator{
		Name: "http",
		CoordinatorConfig: config.CoordinatorConfig{
			Backend:   "http",
			Endpoints: []string{server.URL},
			Interval:  10 * time.Millisecond,
		},
	}
	coord, err := newHTTPCoordinator(def, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to construct http coordinator: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coord.Start(ctx)
	defer coord.Stop()

	waitFor(t, func() bool { return coord.Healthy() })
	if coord.ShouldServePrimary() {
		t.Fatalf("expected veto while allowPrimary=false")
	}

	allow.Store(true)
	waitFor(t, func() bool { return coord.ShouldServePrimary() })
}

func waitFor(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not satisfied before deadline")
}

func boolToString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
