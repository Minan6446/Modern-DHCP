package dhcpv6

import (
	"context"
	"net"
	"testing"

	"modern-dhcp/internal/config"
	"modern-dhcp/internal/mobility"
	"modern-dhcp/internal/relay"

	"go.uber.org/zap"
)

func TestDeviceClassificationUsesFingerprinter(t *testing.T) {
	detector := mobility.NewDetector(config.MobileProfilesConfig{
		Enabled: true,
		Profiles: []config.MobileProfileSpec{
			{
				Name:                "ios",
				Platform:            "ios",
				Persona:             "byod",
				Tags:                []string{"mobile", "ios"},
				VendorClassContains: []string{"iphone"},
			},
		},
	})
	if detector == nil {
		t.Fatalf("expected detector")
	}
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	handler := &Handler{fingerprinter: detector}
	deviceType, persona, tags := handler.deviceClassification(Packet{VendorClass: "iPhone-17", ClientMAC: mac})
	if deviceType != "ios" {
		t.Fatalf("expected ios deviceType, got %q", deviceType)
	}
	if persona != "byod" {
		t.Fatalf("expected persona propagated, got %q", persona)
	}
	if len(tags) != 2 {
		t.Fatalf("expected tags propagated")
	}
}

func TestResolveRelayPoolFromPolicyMetadata(t *testing.T) {
	meta := relay.Metadata{TenantID: "t1", CircuitID: "if-100", RemoteID: "rem-9", VLANID: 300}
	policyMeta := map[string]any{
		"relayPoolRules": []any{
			map[string]any{
				"poolId": "pool-interface",
				"match": map[string]any{
					"tenantId":    "t1",
					"interfaceId": "if-100",
				},
			},
		},
	}

	if got := resolveRelayPoolFromPolicyMetadata(policyMeta, meta); got != "pool-interface" {
		t.Fatalf("expected pool-interface, got %q", got)
	}
}

func TestResolvePoolSelectorUsesRelaySelectorForRemoteID(t *testing.T) {
	h := &Handler{
		relaySel: relay.NewSelector(config.RelayProxyConfig{
			Enabled: true,
			PoolSelection: config.RelayPoolSelectionConfig{
				Rules: []config.RelayPoolRule{
					{
						PoolID: "pool-remote",
						Match: config.RelayMatchCriteria{
							TenantID: "t1",
							RemoteID: "rem-42",
						},
					},
				},
			},
		}, zap.NewNop()),
		logger: zap.NewNop(),
	}

	meta := relay.Metadata{TenantID: "t1", RemoteID: "rem-42", RelayID: "rem-42"}
	got := h.resolvePoolSelector(context.Background(), "t1", nil, meta, nil)
	if got != "pool-remote" {
		t.Fatalf("expected pool-remote from relay selector, got %q", got)
	}
}
