package failover

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

type spyIngress struct {
	roles   []Role
	healthy bool
}

func (s *spyIngress) Start(ctx context.Context, meta config.HANodeMetadata, cfg config.IngressConfig) {
}
func (s *spyIngress) Stop()               {}
func (s *spyIngress) Healthy() bool       { return s.healthy }
func (s *spyIngress) Advertise(role Role) { s.roles = append(s.roles, role) }
func (s *spyIngress) UpdatePolicy(policy IngressPolicy) error {
	return nil
}

type spyBroadcast struct {
	events []struct {
		role  Role
		state State
	}
}

func (s *spyBroadcast) Start(ctx context.Context, cfg config.BroadcastConfig) {}
func (s *spyBroadcast) Stop()                                                 {}
func (s *spyBroadcast) Publish(role Role, state State) {
	s.events = append(s.events, struct {
		role  Role
		state State
	}{role: role, state: state})
}

type spyDiscovery struct {
	healthy   bool
	refreshes []Role
}

func (s *spyDiscovery) Start(ctx context.Context, meta config.HANodeMetadata, cfg config.DiscoveryConfig) {
}
func (s *spyDiscovery) Stop()         {}
func (s *spyDiscovery) Healthy() bool { return s.healthy }

type fakeCoordinator struct {
	healthy bool
	primary bool
}

type fakeReplicator struct {
	healthy   bool
	telemetry ReplicationTelemetry
}

func (f *fakeReplicator) Start(ctx context.Context) {}
func (f *fakeReplicator) Stop()                     {}
func (f *fakeReplicator) Healthy() bool             { return f.healthy }
func (f *fakeReplicator) ReplicationTelemetry() ReplicationTelemetry {
	return f.telemetry
}

func (f *fakeCoordinator) Start(ctx context.Context) {}
func (f *fakeCoordinator) Stop()                     {}
func (f *fakeCoordinator) Healthy() bool             { return f.healthy }
func (f *fakeCoordinator) ShouldServePrimary() bool  { return f.primary }
func (s *spyDiscovery) Refresh(role Role)            { s.refreshes = append(s.refreshes, role) }

func TestManagerRoleChangeNotifiesIngressAndBroadcast(t *testing.T) {
	mgr := NewManager(config.HAConfig{}, zap.NewNop())
	ingress := &spyIngress{healthy: true}
	broadcast := &spyBroadcast{}
	registry := &spyDiscovery{healthy: true}
	mgr.ingress = ingress
	mgr.broadcaster = broadcast
	mgr.discovery = registry

	mgr.setRole(RolePrimary)

	if len(ingress.roles) != 1 || ingress.roles[0] != RolePrimary {
		t.Fatalf("expected ingress advertise primary once, got %v", ingress.roles)
	}
	if len(broadcast.events) != 1 {
		t.Fatalf("expected single broadcast event, got %d", len(broadcast.events))
	}
	ev := broadcast.events[0]
	if ev.role != RolePrimary || ev.state != StateInit {
		t.Fatalf("unexpected broadcast payload: %+v", ev)
	}

	// Second promotion should be ignored.
	mgr.setRole(RolePrimary)
	if len(broadcast.events) != 1 {
		t.Fatalf("expected no additional broadcast on stable role, got %d", len(broadcast.events))
	}

	mgr.setRole(RoleStandby)
	if got := ingress.roles[len(ingress.roles)-1]; got != RoleStandby {
		t.Fatalf("expected standby advertisement, got %s", got)
	}
	if len(broadcast.events) != 2 {
		t.Fatalf("expected second broadcast event for standby, got %d", len(broadcast.events))
	}
}

func TestManagerStateChangePublishesBroadcast(t *testing.T) {
	mgr := NewManager(config.HAConfig{}, zap.NewNop())
	broadcast := &spyBroadcast{}
	mgr.broadcaster = broadcast

	mgr.setState(StateCommInterrupted)
	if len(broadcast.events) != 1 {
		t.Fatalf("expected broadcast for state transition, got %d", len(broadcast.events))
	}
	if broadcast.events[0].state != StateCommInterrupted {
		t.Fatalf("unexpected state recorded: %+v", broadcast.events[0])
	}

	mgr.setState(StateCommInterrupted)
	if len(broadcast.events) != 1 {
		t.Fatalf("duplicate state should not emit event, got %d", len(broadcast.events))
	}

	mgr.setState(StatePartnerDown)
	if len(broadcast.events) != 2 {
		t.Fatalf("expected second event for partner-down, got %d", len(broadcast.events))
	}
	if broadcast.events[1].state != StatePartnerDown {
		t.Fatalf("expected partner-down broadcast, got %+v", broadcast.events[1])
	}
}

func TestManagerRefreshDiscoveryUsesCurrentRole(t *testing.T) {
	mgr := NewManager(config.HAConfig{}, zap.NewNop())
	registry := &spyDiscovery{healthy: true}
	mgr.discovery = registry

	mgr.setRole(RolePrimary)
	mgr.refreshDiscovery()

	if len(registry.refreshes) != 1 {
		t.Fatalf("expected a single discovery refresh, got %d", len(registry.refreshes))
	}
	if registry.refreshes[0] != RolePrimary {
		t.Fatalf("expected discovery refresh with primary role, got %s", registry.refreshes[0])
	}

	mgr.setRole(RoleStandby)
	mgr.refreshDiscovery()
	if got := registry.refreshes[len(registry.refreshes)-1]; got != RoleStandby {
		t.Fatalf("expected standby refresh, got %s", got)
	}
}

func TestManualFailbackRequiresApproval(t *testing.T) {
	cfg := config.HAConfig{
		Failback: config.FailbackConfig{Mode: "manual"},
	}
	mgr := NewManager(cfg, zap.NewNop())
	coord := &fakeCoordinator{healthy: true, primary: false}
	mgr.coordinator = coord
	mgr.setRole(RolePrimary)

	mgr.reconcileCoordinator()
	if mgr.Role() != RolePrimary {
		t.Fatalf("expected manager to remain primary without approval, got %s", mgr.Role())
	}

	mgr.AllowManualFailback()
	mgr.reconcileCoordinator()
	if mgr.Role() != RoleStandby {
		t.Fatalf("expected manager to demote after manual approval, got %s", mgr.Role())
	}
	if mgr.manualFailbackAllowed.Load() {
		t.Fatalf("manual flag should reset after demotion")
	}
}

func TestAutoFailbackWaitsForStability(t *testing.T) {
	cfg := config.HAConfig{
		Failback: config.FailbackConfig{Mode: "auto", StablePeriod: time.Minute},
	}
	mgr := NewManager(cfg, zap.NewNop())
	coord := &fakeCoordinator{healthy: true, primary: false}
	mgr.coordinator = coord
	mgr.setRole(RolePrimary)
	mgr.setState(StateNormal)

	mgr.reconcileCoordinator()
	if mgr.Role() != RolePrimary {
		t.Fatalf("expected to stay primary until stable period elapsed")
	}

	mgr.peerHealthySince = time.Now().Add(-2 * time.Minute)
	mgr.reconcileCoordinator()
	if mgr.Role() != RoleStandby {
		t.Fatalf("expected automatic failback once stable period satisfied, got %s", mgr.Role())
	}
}

func TestSnapshotReflectsState(t *testing.T) {
	mgr := NewManager(config.HAConfig{}, zap.NewNop())
	mgr.setRole(RolePrimary)
	mgr.setState(StateNormal)
	mgr.AllowManualFailback()

	snap := mgr.Snapshot()
	if snap.Role != RolePrimary || snap.State != StateNormal {
		t.Fatalf("unexpected snapshot role/state: %+v", snap)
	}
	if snap.PeerHealthySince.IsZero() {
		t.Fatalf("expected peer healthy timestamp to be recorded")
	}
	if !snap.ManualFailbackSet {
		t.Fatalf("expected manual flag to be reflected in snapshot")
	}
	if snap.FencingEpoch == "" {
		t.Fatalf("expected fencing epoch in snapshot")
	}
}

func TestFencingEpochRotatesOnRoleChange(t *testing.T) {
	mgr := NewManager(config.HAConfig{}, zap.NewNop())
	before := mgr.Snapshot().FencingEpoch
	if before == "" {
		t.Fatalf("expected initial fencing epoch")
	}
	time.Sleep(1 * time.Nanosecond)
	mgr.setRole(RolePrimary)
	after := mgr.Snapshot().FencingEpoch
	if after == "" {
		t.Fatalf("expected fencing epoch after role transition")
	}
	if after == before {
		t.Fatalf("expected fencing epoch to rotate on role change")
	}
}

func TestObserveReplicationPropagatesTelemetryToSnapshot(t *testing.T) {
	mgr := NewManager(config.HAConfig{}, zap.NewNop())
	mgr.replicator = &fakeReplicator{
		healthy: true,
		telemetry: ReplicationTelemetry{
			ApplyLagMs: 17,
			LastOffset: "mysql-bin.000123:4567",
			Healthy:    true,
			LastError:  "",
			UpdatedAt:  time.Now().UTC(),
		},
	}

	mgr.observeReplication()
	snap := mgr.Snapshot()
	if snap.Replication.ApplyLagMs != 17 {
		t.Fatalf("expected replication lag 17ms, got %d", snap.Replication.ApplyLagMs)
	}
	if snap.Replication.LastOffset != "mysql-bin.000123:4567" {
		t.Fatalf("unexpected replication last offset: %s", snap.Replication.LastOffset)
	}
	if !snap.Replication.Healthy {
		t.Fatalf("expected replication healthy in snapshot")
	}
}
