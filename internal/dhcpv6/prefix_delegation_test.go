package dhcpv6

import (
	"testing"

	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/policy"
	"modern-dhcp/pkg/models"
)

func TestStaticPrefixFromPolicy(t *testing.T) {
	decision := &policy.Decision{Metadata: map[string]any{"staticPrefix": "2001:db8:100:10::/64"}}
	pd, ok := staticPrefixFromPolicy(decision)
	if !ok {
		t.Fatalf("expected static prefix from policy metadata")
	}
	if pd.Prefix != "2001:db8:100:10::/64" || pd.Reason != "static-policy" {
		t.Fatalf("unexpected policy static prefix decision: %+v", pd)
	}
}

func TestStaticPrefixFromBinding(t *testing.T) {
	meta := []byte(`{"pdPrefix":"2001:db8:100:20::/64"}`)
	pd, ok := staticPrefixFromBinding(meta)
	if !ok {
		t.Fatalf("expected static prefix from binding metadata")
	}
	if pd.Prefix != "2001:db8:100:20::/64" || pd.Reason != "static-binding" {
		t.Fatalf("unexpected binding static prefix decision: %+v", pd)
	}
}

func TestEncodeIAPDOptionUsesPoolPrefixFallback(t *testing.T) {
	pd := &lease.PrefixDelegation{IAPDID: 10, PrefixLength: 64}
	poolObj := &models.AddressPool{CIDR: "2001:db8:100::/48"}
	payload, err := encodeIAPDOption(pd, poolObj, 100, 200, 3600, 7200)
	if err != nil {
		t.Fatalf("encodeIAPDOption failed: %v", err)
	}
	if len(payload) == 0 {
		t.Fatalf("expected non-empty IAPD payload")
	}
}
