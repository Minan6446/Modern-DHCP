package lease

import (
	"errors"
	"testing"

	"modern-dhcp/internal/failover"
)

type failoverReporterStub struct {
	snapshot failover.StatusSnapshot
}

func (f failoverReporterStub) Snapshot() failover.StatusSnapshot {
	return f.snapshot
}

func TestEnsurePrimaryWriteAllowsPrimaryAndUnknown(t *testing.T) {
	svcPrimary := &Service{failoverStatus: failoverReporterStub{snapshot: failover.StatusSnapshot{Role: failover.RolePrimary, State: failover.StateNormal}}}
	if err := svcPrimary.ensurePrimaryWrite("lease.allocate_or_reuse"); err != nil {
		t.Fatalf("expected primary role to pass guard, got %v", err)
	}

	svcUnknown := &Service{failoverStatus: failoverReporterStub{snapshot: failover.StatusSnapshot{Role: failover.RoleUnknown, State: failover.StateInit}}}
	if err := svcUnknown.ensurePrimaryWrite("lease.allocate_or_reuse"); err != nil {
		t.Fatalf("expected unknown role to pass guard, got %v", err)
	}
}

func TestEnsurePrimaryWriteBlocksStandby(t *testing.T) {
	svc := &Service{failoverStatus: failoverReporterStub{snapshot: failover.StatusSnapshot{Role: failover.RoleStandby, State: failover.StateNormal}}}
	err := svc.ensurePrimaryWrite("lease.allocate_or_reuse")
	if err == nil {
		t.Fatalf("expected standby role to be blocked")
	}
	if !errors.Is(err, ErrPrimaryRequired) {
		t.Fatalf("expected ErrPrimaryRequired, got %v", err)
	}
}
