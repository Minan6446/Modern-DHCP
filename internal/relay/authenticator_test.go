package relay

import (
	"net"
	"testing"

	"modern-dhcp/internal/config"
)

func TestAuthenticatorAllowlist(t *testing.T) {
	cfg := config.RelayAuthenticationConfig{
		Required: true,
		AllowedAgents: []config.RelayAgentIdentity{
			{RelayID: "agg-core", VLANRanges: []string{"100-200"}},
		},
	}
	auth := NewAuthenticator(cfg, nil)
	meta := BuildMetadata("tenant", net.ParseIP("10.0.0.1"), map[string]string{"remote-id": "agg-core"}, 150, nil)
	if err := auth.Authorize(meta); err != nil {
		t.Fatalf("expected allowlist match, got %v", err)
	}
}

func TestAuthenticatorSecretMismatch(t *testing.T) {
	cfg := config.RelayAuthenticationConfig{
		Required: true,
		SharedSecrets: map[string]string{
			"relay-1": "secret",
		},
	}
	auth := NewAuthenticator(cfg, nil)
	meta := BuildMetadata("tenant", net.ParseIP("10.0.0.2"), map[string]string{"remote-id": "relay-1", "auth-token": "bad"}, 0, nil)
	if err := auth.Authorize(meta); err == nil {
		t.Fatalf("expected secret mismatch error")
	}
}
