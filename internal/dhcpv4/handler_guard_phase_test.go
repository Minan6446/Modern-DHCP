package dhcpv4

import (
	"context"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	securityguard "modern-dhcp/internal/security/guard"
)

type phaseGuardStub struct {
	checkCalls        int
	renewCalls        int
	lastCheckContext  *securityguard.Context
	lastRenewMAC      string
	lastRenewTenantID string
}

func (s *phaseGuardStub) Check(ctx context.Context, ctxData *securityguard.Context) error {
	s.checkCalls++
	s.lastCheckContext = ctxData
	return nil
}

func (s *phaseGuardStub) CheckRenewACL(ctx context.Context, mac string) error {
	s.renewCalls++
	s.lastRenewMAC = mac
	s.lastRenewTenantID = lease.TenantIDFromContext(ctx)
	return nil
}

func (s *phaseGuardStub) OnLeaseChange(ctx context.Context, change *securityguard.LeaseChange) {}

func (s *phaseGuardStub) OnSecurityEvent(ctx context.Context, evt securityguard.Event) {}

func TestRunGuardRequestPhaseUsesStandardCheck(t *testing.T) {
	stub := &phaseGuardStub{}
	h := &Handler{guard: stub, logger: zap.NewNop()}
	pkt := Packet{
		CHAddr:         net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		ClientID:       "cid-1",
		RelayAgentInfo: map[string]string{"port-id": "ge-0/0/1"},
		RequestedIP:    net.IPv4(10, 0, 0, 10),
		GIAddr:         net.IPv4(10, 0, 0, 1),
	}

	if err := h.runGuard(context.Background(), "tenant-a", "request", pkt, dhcpv4RequestPhaseRequest); err != nil {
		t.Fatalf("runGuard error: %v", err)
	}
	if stub.checkCalls != 1 {
		t.Fatalf("expected Check to be called once, got %d", stub.checkCalls)
	}
	if stub.renewCalls != 0 {
		t.Fatalf("expected CheckRenewACL not to be called, got %d", stub.renewCalls)
	}
	if stub.lastCheckContext == nil || stub.lastCheckContext.MAC != "00:11:22:33:44:55" {
		t.Fatalf("expected Check context MAC to be propagated")
	}
}

func TestRunGuardRenewAndRebindUseRenewCheck(t *testing.T) {
	stub := &phaseGuardStub{}
	h := &Handler{guard: stub, logger: zap.NewNop()}
	pkt := Packet{CHAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}

	if err := h.runGuard(context.Background(), "tenant-r", "request", pkt, dhcpv4RequestPhaseRenew); err != nil {
		t.Fatalf("runGuard renew error: %v", err)
	}
	if err := h.runGuard(context.Background(), "tenant-r", "request", pkt, dhcpv4RequestPhaseRebind); err != nil {
		t.Fatalf("runGuard rebind error: %v", err)
	}

	if stub.checkCalls != 0 {
		t.Fatalf("expected Check not to be called for renew/rebind, got %d", stub.checkCalls)
	}
	if stub.renewCalls != 2 {
		t.Fatalf("expected CheckRenewACL to be called twice, got %d", stub.renewCalls)
	}
	if stub.lastRenewMAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("expected renew MAC to be propagated, got %s", stub.lastRenewMAC)
	}
	if stub.lastRenewTenantID != "tenant-r" {
		t.Fatalf("expected tenant context to be set for renew path, got %s", stub.lastRenewTenantID)
	}
}

func TestRunGuardNoopWhenGuardNil(t *testing.T) {
	h := &Handler{guard: nil, logger: zap.NewNop()}
	pkt := Packet{CHAddr: net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, RequestedIP: net.IPv4(10, 0, 0, 2), GIAddr: net.IPv4(10, 0, 0, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.runGuard(ctx, "tenant-z", "discover", pkt, dhcpv4RequestPhaseRequest); err != nil {
		t.Fatalf("expected nil error when guard is nil, got %v", err)
	}
}
