package dhcpv6

import (
	"net"
	"testing"

	"go.uber.org/zap"

	dhcpv6fsm "modern-dhcp/internal/dhcpv6/leasefsm"
	"modern-dhcp/internal/lease"
	"modern-dhcp/pkg/models"
)

func TestConfirmAddressWithinPool(t *testing.T) {
	if !confirmAddressWithinPool("2001:db8:1::/64", net.ParseIP("2001:db8:1::10")) {
		t.Fatalf("expected address to be within pool")
	}
	if confirmAddressWithinPool("2001:db8:1::/64", net.ParseIP("2001:db8:2::10")) {
		t.Fatalf("expected address outside pool to be rejected")
	}
}

func TestZeroLifetimeConfirmResult(t *testing.T) {
	pkt := Packet{IAAddr: net.ParseIP("2001:db8:1::99")}
	res := zeroLifetimeConfirmResult(pkt)
	if res == nil || res.Lease == nil {
		t.Fatalf("expected confirm result lease payload")
	}
	if res.Lease.IPAddress != "2001:db8:1::99" {
		t.Fatalf("unexpected lease ip %q", res.Lease.IPAddress)
	}
	if res.Profile.DefaultDuration != 0 || res.Profile.MaxDuration != 0 {
		t.Fatalf("expected zero lifetimes in confirm reject result")
	}
}

func TestIdempotentRenewRequest(t *testing.T) {
	pkt := Packet{IAAddr: net.ParseIP("2001:db8:1::80")}
	result := &lease.Result{Reused: true, Lease: &models.Lease{IPAddress: "2001:db8:1::80"}}
	if !isIdempotentRenewRequest(pkt, result) {
		t.Fatalf("expected renew request to be idempotent")
	}
	result.Lease.IPAddress = "2001:db8:1::81"
	if isIdempotentRenewRequest(pkt, result) {
		t.Fatalf("expected renew request with different address to be non-idempotent")
	}
}

func TestServerTransitionLeaseState(t *testing.T) {
	srv := NewServer(Options{TenantID: "t1", ServerID: []byte{0, 1, 2, 3}}, nil, zap.NewNop())
	key := "duid:test-1"
	if err := srv.transitionLeaseState(key, dhcpv6fsm.LeaseStateOffered); err != nil {
		t.Fatalf("init->offered transition failed: %v", err)
	}
	if err := srv.transitionLeaseState(key, dhcpv6fsm.LeaseStateBound); err != nil {
		t.Fatalf("offered->bound transition failed: %v", err)
	}
}

func TestServerTransitionLeaseStateRejectsInvalid(t *testing.T) {
	srv := NewServer(Options{TenantID: "t1", ServerID: []byte{0, 1, 2, 3}}, nil, zap.NewNop())
	key := "duid:test-2"
	if err := srv.transitionLeaseState(key, dhcpv6fsm.LeaseStateOffered); err != nil {
		t.Fatalf("init->offered transition failed: %v", err)
	}
	if err := srv.transitionLeaseState(key, dhcpv6fsm.LeaseStateReleased); err == nil {
		t.Fatalf("expected offered->released to be rejected")
	}
}
