package relay

import (
	"testing"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

func TestSelectorGiaddrRule(t *testing.T) {
	cfg := config.RelayProxyConfig{
		Enabled: true,
		PoolSelection: config.RelayPoolSelectionConfig{
			GiaddrRules: []config.RelayGiaddrRule{
				{TenantID: "tenant-a", GIAddr: "10.0.0.1", PoolID: "pool-edge"},
			},
		},
	}
	selector := NewSelector(cfg, zap.NewNop())
	pool, ok := selector.Resolve(Metadata{TenantID: "tenant-a", GIAddr: "10.0.0.1"})
	if !ok || pool != "pool-edge" {
		t.Fatalf("expected giaddr match, got %s %v", pool, ok)
	}
}

func TestSelectorVLANRange(t *testing.T) {
	cfg := config.RelayProxyConfig{
		Enabled: true,
		CrossDomain: config.RelayCrossDomainConfig{
			MultiVLANPools: []config.RelayVLANPool{
				{PoolID: "pool-vlan", VLANs: []string{"100-102"}},
			},
		},
	}
	selector := NewSelector(cfg, zap.NewNop())
	pool, ok := selector.Resolve(Metadata{TenantID: "t1", VLANID: 101})
	if !ok || pool != "pool-vlan" {
		t.Fatalf("expected vlan pool match, got %s %v", pool, ok)
	}
}

func TestSelectorRegionFallback(t *testing.T) {
	cfg := config.RelayProxyConfig{
		Enabled: true,
		CrossDomain: config.RelayCrossDomainConfig{
			RegionFallbacks: map[string][]string{
				"dc-west": {"pool-west"},
			},
		},
	}
	selector := NewSelector(cfg, zap.NewNop())
	pool, ok := selector.Resolve(Metadata{TenantID: "t1", Region: "DC-West"})
	if !ok || pool != "pool-west" {
		t.Fatalf("expected region fallback, got %s %v", pool, ok)
	}
}
