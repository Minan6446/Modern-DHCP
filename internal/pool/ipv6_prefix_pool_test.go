package pool

import (
	"net/netip"
	"testing"
)

func TestIPv6PrefixPoolDynamicAllocation(t *testing.T) {
	poolPrefix := netip.MustParsePrefix("2001:db8:100::/48")
	allocator, err := NewIPv6PrefixPool(poolPrefix, 64, nil)
	if err != nil {
		t.Fatalf("NewIPv6PrefixPool failed: %v", err)
	}
	prefix, ok := allocator.AllocateAvailablePrefix(netip.Prefix{})
	if !ok {
		t.Fatalf("expected dynamic prefix allocation")
	}
	if prefix.Bits() != 64 {
		t.Fatalf("expected delegated /64, got /%d", prefix.Bits())
	}
	if !poolPrefix.Contains(prefix.Addr()) {
		t.Fatalf("allocated prefix %s outside pool %s", prefix, poolPrefix)
	}
}

func TestIPv6PrefixPoolStaticConflictRejected(t *testing.T) {
	poolPrefix := netip.MustParsePrefix("2001:db8:100::/48")
	used := map[string]struct{}{"2001:db8:100::/64": {}}
	allocator, err := NewIPv6PrefixPool(poolPrefix, 64, used)
	if err != nil {
		t.Fatalf("NewIPv6PrefixPool failed: %v", err)
	}
	staticPrefix := netip.MustParsePrefix("2001:db8:100::/64")
	if _, ok := allocator.AllocateSpecificPrefix(staticPrefix); ok {
		t.Fatalf("expected static conflicted prefix allocation to be rejected")
	}
}
