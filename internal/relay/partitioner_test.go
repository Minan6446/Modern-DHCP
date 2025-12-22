package relay

import (
	"net"
	"testing"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

func TestPartitionerConsistentHash(t *testing.T) {
	cfg := config.RelayLoadBalancingConfig{
		Mode: "consistent-hash",
		Servers: []config.RelayServerConfig{
			{ID: "node-a", Weight: 1},
			{ID: "node-b", Weight: 1},
		},
		HashKeys: []string{"relayId"},
	}
	pA := NewPartitioner(cfg, "node-a", zap.NewNop())
	pB := NewPartitioner(cfg, "node-b", zap.NewNop())
	meta := BuildMetadata("tenant", net.ParseIP("10.0.0.1"), map[string]string{"remote-id": "relay-x"}, 0, nil)
	if pA.ShouldHandle(meta) == pB.ShouldHandle(meta) {
		t.Fatalf("expected different owners between nodes")
	}
}
