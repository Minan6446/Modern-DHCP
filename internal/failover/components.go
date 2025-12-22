package failover

import (
	"context"
	"sync/atomic"
	"time"

	"modern-dhcp/internal/config"
)

// Coordinator mediates cluster-wide leadership decisions (etcd/RAFT/Consul wrapper).
type Coordinator interface {
	Start(ctx context.Context)
	Stop()
	Healthy() bool
	ShouldServePrimary() bool
}

// Replicator streams lease/binlog updates to warm standby nodes.
type Replicator interface {
	Start(ctx context.Context)
	Stop()
	Healthy() bool
}

// ConfigWatcher validates GitOps/etcd snapshots and exposes checksums.
type ConfigWatcher interface {
	Start(ctx context.Context)
	Stop()
	LastChecksum() string
}

// HeartbeatTransport manages TCP heartbeat/failover links.
type HeartbeatTransport interface {
	Start(ctx context.Context, handler func())
	Stop()
}

// IngressController manages client-facing entrypoints such as VIP/DNS/anycast.
type IngressController interface {
	Start(ctx context.Context, meta config.HANodeMetadata, cfg config.IngressConfig)
	Stop()
	Healthy() bool
	Advertise(role Role)
	UpdatePolicy(policy IngressPolicy) error
}

// BroadcastBus emits role/state events for observers syncing control planes.
type BroadcastBus interface {
	Start(ctx context.Context, cfg config.BroadcastConfig)
	Stop()
	Publish(role Role, state State)
}

// DiscoveryRegistry registers the node for peer/agent discovery.
type DiscoveryRegistry interface {
	Start(ctx context.Context, meta config.HANodeMetadata, cfg config.DiscoveryConfig)
	Stop()
	Healthy() bool
	Refresh(role Role)
}

// Noop implementations keep the manager operational when external systems
// are not yet wired up.
type noopCoordinator struct{}

type noopReplicator struct{}

type noopConfigWatcher struct {
	checksum atomic.Value
}

type noopHeartbeat struct{}

type noopIngress struct{}

type noopBroadcast struct{}

type noopDiscovery struct{}

func (noopCoordinator) Start(ctx context.Context) {}
func (noopCoordinator) Stop()                     {}
func (noopCoordinator) Healthy() bool             { return true }
func (noopCoordinator) ShouldServePrimary() bool  { return true }

func (noopReplicator) Start(ctx context.Context) {}
func (noopReplicator) Stop()                     {}
func (noopReplicator) Healthy() bool             { return true }

func (n *noopConfigWatcher) Start(ctx context.Context) {}
func (n *noopConfigWatcher) Stop()                     {}
func (n *noopConfigWatcher) LastChecksum() string {
	if n == nil {
		return ""
	}
	if v := n.checksum.Load(); v != nil {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

func (n *noopConfigWatcher) setChecksum(value string) {
	if n == nil {
		return
	}
	n.checksum.Store(value)
}

func (noopHeartbeat) Start(ctx context.Context, handler func()) {}
func (noopHeartbeat) Stop()                                     {}

func (noopIngress) Start(ctx context.Context, meta config.HANodeMetadata, cfg config.IngressConfig) {}
func (noopIngress) Stop()                                                                           {}
func (noopIngress) Healthy() bool                                                                   { return true }
func (noopIngress) Advertise(role Role)                                                             {}
func (noopIngress) UpdatePolicy(policy IngressPolicy) error                                         { return nil }

func (noopBroadcast) Start(ctx context.Context, cfg config.BroadcastConfig) {}
func (noopBroadcast) Stop()                                                 {}
func (noopBroadcast) Publish(role Role, state State)                        {}

func (noopDiscovery) Start(ctx context.Context, meta config.HANodeMetadata, cfg config.DiscoveryConfig) {
}
func (noopDiscovery) Stop()             {}
func (noopDiscovery) Healthy() bool     { return true }
func (noopDiscovery) Refresh(role Role) {}

// Utility helpers -----------------------------------------------------------

func defaultCoordinator() Coordinator { return noopCoordinator{} }

func defaultReplicator() Replicator { return noopReplicator{} }

func defaultConfigWatcher() *noopConfigWatcher { return &noopConfigWatcher{} }

func defaultHeartbeat() HeartbeatTransport { return noopHeartbeat{} }

func durationOrDefault(v, fallback time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return fallback
}
