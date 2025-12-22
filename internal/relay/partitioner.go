package relay

import (
	"hash/fnv"
	"strings"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// Partitioner decides if the current node should handle a relay request.
type Partitioner struct {
	nodeID   string
	ring     []string
	hashKeys []string
	logger   *zap.Logger
}

// NewPartitioner builds a consistent hashing partitioner when servers are defined.
func NewPartitioner(cfg config.RelayLoadBalancingConfig, nodeID string, logger *zap.Logger) *Partitioner {
	if len(cfg.Servers) == 0 || strings.TrimSpace(nodeID) == "" {
		return nil
	}
	ring := buildServerRing(cfg.Servers)
	if len(ring) == 0 {
		return nil
	}
	keys := cfg.HashKeys
	if len(keys) == 0 {
		keys = []string{"relayId", "giaddr"}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Partitioner{nodeID: nodeID, ring: ring, hashKeys: keys, logger: logger}
}

// ShouldHandle returns true when the current node owns the relay hash.
func (p *Partitioner) ShouldHandle(meta Metadata) bool {
	if p == nil {
		return true
	}
	if len(p.ring) == 0 {
		return true
	}
	key := p.hashInput(meta)
	if key == "" {
		return true
	}
	idx := int(fnvHash(key) % uint64(len(p.ring)))
	owner := p.ring[idx]
	if owner == p.nodeID {
		return true
	}
	p.logger.Debug("relay partition redirect", zap.String("relayId", meta.RelayID), zap.String("owner", owner), zap.String("nodeId", p.nodeID))
	return false
}

func (p *Partitioner) hashInput(meta Metadata) string {
	var parts []string
	for _, key := range p.hashKeys {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "relayid":
			parts = append(parts, meta.RelayID)
		case "giaddr":
			parts = append(parts, meta.GIAddr)
		case "circuitid":
			parts = append(parts, meta.CircuitID)
		case "remoteid":
			parts = append(parts, meta.RemoteID)
		}
	}
	if len(parts) == 0 {
		return meta.RelayID
	}
	return strings.Join(parts, "|")
}

func buildServerRing(servers []config.RelayServerConfig) []string {
	if len(servers) == 0 {
		return nil
	}
	var ring []string
	for _, server := range servers {
		weight := server.Weight
		if weight <= 0 {
			weight = 1
		}
		for i := 0; i < weight; i++ {
			ring = append(ring, strings.TrimSpace(server.ID))
		}
	}
	return ring
}

func fnvHash(data string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(data))
	return h.Sum64()
}
