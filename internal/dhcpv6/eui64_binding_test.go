package dhcpv6

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
)

type fakeDHCPv6LeaseService struct {
	bindingsByMAC  map[string]lease.IPv6EUI64Binding
	bindingsByAddr map[string]lease.IPv6EUI64Binding
	created        int
	lastAllocIP    string
}

func (f *fakeDHCPv6LeaseService) key(mac, prefix string) string {
	return mac + "|" + prefix
}

func (f *fakeDHCPv6LeaseService) AllocateOrReuseWithMetadata(_ context.Context, _ lease.ResourceScope, identifier string, profile models.LeaseProfile, poolID, ip string, _ lease.AllocationMetadata) (*lease.Result, error) {
	f.lastAllocIP = ip
	return &lease.Result{Lease: &models.Lease{ID: "l1", TenantID: "t1", ClientID: identifier, HardwareAddr: identifier, PoolID: poolID, IPAddress: ip, State: "ACTIVE", ExpiresAt: time.Now().UTC().Add(365 * 24 * time.Hour)}, Profile: profile, Pool: &models.AddressPool{ID: poolID, CIDR: "2001:db8:1::/64"}}, nil
}

func (f *fakeDHCPv6LeaseService) AllocateOrReusePrefixes(context.Context, lease.ResourceScope, string, models.LeaseProfile, *models.AddressPool, []lease.PrefixRequest) ([]lease.PrefixDelegation, error) {
	return nil, nil
}

func (f *fakeDHCPv6LeaseService) ReleasePrefix(context.Context, lease.ResourceScope, string, uint32) error {
	return nil
}

func (f *fakeDHCPv6LeaseService) DeclinePrefix(context.Context, lease.ResourceScope, string, uint32) error {
	return nil
}

func (f *fakeDHCPv6LeaseService) ListPrefixLeases(context.Context, lease.ResourceScope, string, int, int) ([]models.PrefixLease, error) {
	return nil, nil
}

func (f *fakeDHCPv6LeaseService) FindIPv6EUI64BindingByMAC(_ context.Context, _ lease.ResourceScope, mac, prefix string) (*lease.IPv6EUI64Binding, error) {
	if f.bindingsByMAC == nil {
		return nil, lease.ErrNotFound
	}
	b, ok := f.bindingsByMAC[f.key(mac, prefix)]
	if !ok {
		return nil, lease.ErrNotFound
	}
	copyVal := b
	return &copyVal, nil
}

func (f *fakeDHCPv6LeaseService) FindIPv6EUI64BindingByIPv6Addr(_ context.Context, _ lease.ResourceScope, _ string, ipv6Addr string) (*lease.IPv6EUI64Binding, error) {
	if f.bindingsByAddr == nil {
		return nil, lease.ErrNotFound
	}
	b, ok := f.bindingsByAddr[ipv6Addr]
	if !ok {
		return nil, lease.ErrNotFound
	}
	copyVal := b
	return &copyVal, nil
}

func (f *fakeDHCPv6LeaseService) CreateIPv6EUI64Binding(_ context.Context, _ lease.ResourceScope, binding lease.IPv6EUI64Binding) (*lease.IPv6EUI64Binding, error) {
	f.created++
	if f.bindingsByMAC == nil {
		f.bindingsByMAC = make(map[string]lease.IPv6EUI64Binding)
	}
	if f.bindingsByAddr == nil {
		f.bindingsByAddr = make(map[string]lease.IPv6EUI64Binding)
	}
	f.bindingsByMAC[f.key(binding.MAC, binding.Prefix)] = binding
	f.bindingsByAddr[binding.IPv6Addr] = binding
	return &binding, nil
}

type fakeDHCPv6PoolService struct{}

func (f *fakeDHCPv6PoolService) ResolvePool(context.Context, pool.ResourceScope, pool.MetadataSelector) (*models.AddressPool, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDHCPv6PoolService) GetPool(context.Context, pool.ResourceScope, string) (*models.AddressPool, error) {
	return &models.AddressPool{ID: "default-v6", CIDR: "2001:db8:1::/64", LeaseProfileID: "lp-v6"}, nil
}

func (f *fakeDHCPv6PoolService) GetLeaseProfile(context.Context, pool.ResourceScope, string) (*models.LeaseProfile, error) {
	return &models.LeaseProfile{DefaultDuration: 3600 * time.Second, MaxDuration: 7200 * time.Second, RenewalTime: 1800 * time.Second, RebindingTime: 3150 * time.Second}, nil
}

func (f *fakeDHCPv6PoolService) FindBinding(context.Context, pool.ResourceScope, string, string) (*models.StaticBinding, error) {
	return nil, pool.ErrNotFound
}

func mustMAC(t *testing.T, raw string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(raw)
	if err != nil {
		t.Fatalf("parse mac: %v", err)
	}
	return mac
}

func TestHandleRequestEUI64_NewMACCreatesBindingAndAllocates(t *testing.T) {
	leaseSvc := &fakeDHCPv6LeaseService{}
	h := &Handler{
		leaseSvc:         leaseSvc,
		poolSvc:          &fakeDHCPv6PoolService{},
		logger:           zap.NewNop(),
		eui64Cache:       make(map[string]lease.IPv6EUI64Binding),
		ipv6ProbeTimeout: time.Second,
		ipv6ConflictFn: func(context.Context, string, time.Duration) (bool, error) {
			return false, nil
		},
	}
	pkt := Packet{DUID: "duid-a", ClientMAC: mustMAC(t, "aa:bb:cc:dd:ee:ff")}
	res, err := h.HandleRequest(context.Background(), "t1", pkt)
	if err != nil {
		t.Fatalf("handle request error: %v", err)
	}
	if res == nil || res.Lease == nil {
		t.Fatalf("expected lease result")
	}
	if leaseSvc.created != 1 {
		t.Fatalf("expected one binding creation, got %d", leaseSvc.created)
	}
	prefix := netip.MustParsePrefix("2001:db8:1::/64")
	expected, _ := DeriveIPv6FromEUI64(prefix, pkt.ClientMAC)
	if res.Lease.IPAddress != expected.String() {
		t.Fatalf("expected allocated eui64 address %s, got %s", expected.String(), res.Lease.IPAddress)
	}
}

func TestHandleRequestEUI64_ExistingBindingReused(t *testing.T) {
	binding := lease.IPv6EUI64Binding{MAC: "aa:bb:cc:dd:ee:ff", Prefix: "2001:db8:1::/64", IPv6Addr: "2001:db8:1::a8bb:ccff:fedd:eeff", LeaseTime: 365 * 24 * time.Hour}
	leaseSvc := &fakeDHCPv6LeaseService{bindingsByMAC: map[string]lease.IPv6EUI64Binding{"aa:bb:cc:dd:ee:ff|2001:db8:1::/64": binding}, bindingsByAddr: map[string]lease.IPv6EUI64Binding{binding.IPv6Addr: binding}}
	h := &Handler{leaseSvc: leaseSvc, poolSvc: &fakeDHCPv6PoolService{}, logger: zap.NewNop(), eui64Cache: make(map[string]lease.IPv6EUI64Binding), ipv6ProbeTimeout: time.Second, ipv6ConflictFn: func(context.Context, string, time.Duration) (bool, error) { return false, nil }}
	res, err := h.HandleRequest(context.Background(), "t1", Packet{DUID: "duid-a", ClientMAC: mustMAC(t, "aa:bb:cc:dd:ee:ff")})
	if err != nil {
		t.Fatalf("handle request error: %v", err)
	}
	if res.Lease.IPAddress != binding.IPv6Addr {
		t.Fatalf("expected existing binding ip %s, got %s", binding.IPv6Addr, res.Lease.IPAddress)
	}
	if leaseSvc.created != 0 {
		t.Fatalf("expected no new binding creation")
	}
}

func TestHandleRequestEUI64_NonBindingMACRequestBoundAddrNACK(t *testing.T) {
	owner := lease.IPv6EUI64Binding{MAC: "aa:bb:cc:dd:ee:11", Prefix: "2001:db8:1::/64", IPv6Addr: "2001:db8:1::a8bb:ccff:fedd:ee11", LeaseTime: 365 * 24 * time.Hour}
	leaseSvc := &fakeDHCPv6LeaseService{bindingsByAddr: map[string]lease.IPv6EUI64Binding{owner.IPv6Addr: owner}}
	h := &Handler{leaseSvc: leaseSvc, poolSvc: &fakeDHCPv6PoolService{}, logger: zap.NewNop(), eui64Cache: make(map[string]lease.IPv6EUI64Binding), ipv6ProbeTimeout: time.Second, ipv6ConflictFn: func(context.Context, string, time.Duration) (bool, error) { return false, nil }}
	pkt := Packet{DUID: "duid-b", ClientMAC: mustMAC(t, "aa:bb:cc:dd:ee:22"), IAAddr: net.ParseIP(owner.IPv6Addr)}
	_, err := h.HandleRequest(context.Background(), "t1", pkt)
	if err == nil {
		t.Fatalf("expected nack error")
	}
	var nack *NACKError
	if !errors.As(err, &nack) {
		t.Fatalf("expected NACKError, got %T", err)
	}
}

func TestHandleRequestEUI64_ConflictDetectedNACK(t *testing.T) {
	leaseSvc := &fakeDHCPv6LeaseService{}
	h := &Handler{
		leaseSvc:         leaseSvc,
		poolSvc:          &fakeDHCPv6PoolService{},
		logger:           zap.NewNop(),
		eui64Cache:       make(map[string]lease.IPv6EUI64Binding),
		ipv6ProbeTimeout: time.Second,
		ipv6ConflictFn: func(context.Context, string, time.Duration) (bool, error) {
			return true, nil
		},
	}
	_, err := h.HandleRequest(context.Background(), "t1", Packet{DUID: "duid-c", ClientMAC: mustMAC(t, "aa:bb:cc:dd:ee:33")})
	if err == nil {
		t.Fatalf("expected nack on conflict")
	}
	var nack *NACKError
	if !errors.As(err, &nack) {
		t.Fatalf("expected NACKError, got %T", err)
	}
	if leaseSvc.created != 0 {
		t.Fatalf("conflict should not create binding")
	}
}

func TestHandleRequestEUI64_UsesPoolLeaseProfile(t *testing.T) {
	leaseSvc := &fakeDHCPv6LeaseService{}
	h := &Handler{
		leaseSvc:         leaseSvc,
		poolSvc:          &fakeDHCPv6PoolService{},
		logger:           zap.NewNop(),
		eui64Cache:       make(map[string]lease.IPv6EUI64Binding),
		ipv6ProbeTimeout: time.Second,
		ipv6ConflictFn: func(context.Context, string, time.Duration) (bool, error) {
			return false, nil
		},
	}
	res, err := h.HandleRequest(context.Background(), "t1", Packet{DUID: "duid-profile", ClientMAC: mustMAC(t, "aa:bb:cc:dd:ee:44")})
	if err != nil {
		t.Fatalf("handle request error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result")
	}
	if res.Profile.DefaultDuration != 3600*time.Second {
		t.Fatalf("expected default duration from pool lease profile, got %v", res.Profile.DefaultDuration)
	}
	if res.Profile.MaxDuration != 7200*time.Second {
		t.Fatalf("expected max duration from pool lease profile, got %v", res.Profile.MaxDuration)
	}
}
