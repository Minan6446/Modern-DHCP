package metrics

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Collector exposes Prometheus metrics used by Modern-DHCP runtime.
type Collector struct {
	HTTPRequests            *prometheus.CounterVec
	HTTPRequestLatency      *prometheus.HistogramVec
	HTTPErrors              *prometheus.CounterVec
	LeaseEvents             *prometheus.CounterVec
	LeaseConflicts          *prometheus.CounterVec
	DHCPv4ACDConflicts      *prometheus.CounterVec
	DHCPv6EUI64Binding      *prometheus.CounterVec
	DHCPv6IPv6Conflicts     *prometheus.CounterVec
	DHCPv4Packets           *prometheus.CounterVec
	DHCPv4LeaseOpDuration   *prometheus.HistogramVec
	PolicyHits              *prometheus.CounterVec
	PoolSelectorResolutions *prometheus.CounterVec
	PoolSelectorErrors      *prometheus.CounterVec
	PoolSelectorLatency     *prometheus.HistogramVec
	PoolMetadataSnapshot    *prometheus.GaugeVec
	PoolServiceLatency      *prometheus.HistogramVec
	SecurityEvents          *prometheus.CounterVec
	SecurityGuardLatency    *prometheus.HistogramVec
	SnoopingCache           *prometheus.CounterVec
	DHCPRequestLifecycle    *prometheus.CounterVec
	DHCPRequestLatency      *prometheus.HistogramVec
	LeaseReplicationLag     *prometheus.HistogramVec
	LeaseSyncAckFailures    *prometheus.CounterVec
	SyncTxnReconcileRuns    *prometheus.CounterVec
	LeaseAllocatorLatency   *prometheus.HistogramVec
	LeaseAllocatorAttempts  *prometheus.CounterVec
	PoolUtilization         *prometheus.GaugeVec
	DBPoolStats             *prometheus.GaugeVec
	CacheEvents             *prometheus.CounterVec
	MobilityAffinityHits    *prometheus.CounterVec
	HARole                  *prometheus.GaugeVec
	HAState                 *prometheus.GaugeVec
	HAPeerGapSeconds        prometheus.Gauge
	HAFailoverTransitions   *prometheus.CounterVec
	HACoordinatorQuorum     *prometheus.GaugeVec
}

var namespaceSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

// NewCollector registers metrics with the default registry.
func NewCollector(namespace string) *Collector {
	namespace = sanitizeNamespace(namespace)
	return &Collector{
		HTTPRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Count of management API requests",
		}, []string{"endpoint", "method", "status"}),
		HTTPRequestLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "Latency of management API requests",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"endpoint", "method", "status"}),
		HTTPErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_errors_total",
			Help:      "Count of management API responses considered errors (>=500)",
		}, []string{"endpoint", "method", "status"}),
		LeaseEvents: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "lease_events_total",
			Help:      "Count of lease lifecycle events",
		}, []string{"action"}),
		LeaseConflicts: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "lease_conflicts_total",
			Help:      "Count of lease conflict detections",
		}, []string{"tenant", "signal"}),
		DHCPv4ACDConflicts: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dhcpv4_acd_conflict_total",
			Help: "Count of DHCPv4 ACD conflicts detected during IP allocation",
		}, []string{"pool"}),
		DHCPv6EUI64Binding: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dhcpv6_eui64_binding_total",
			Help: "Count of DHCPv6 EUI-64 binding operations grouped by result",
		}, []string{"result"}),
		DHCPv6IPv6Conflicts: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dhcpv6_ipv6_conflict_total",
			Help: "Count of DHCPv6 IPv6 address conflicts detected by ICMPv6 probing",
		}, []string{"result"}),
		DHCPv4Packets: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "dhcpv4_packet_total",
			Help: "Count of DHCPv4 packets by message type",
		}, []string{"type"}),
		DHCPv4LeaseOpDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dhcpv4_lease_operation_duration_seconds",
			Help:    "Duration of DHCPv4 lease operations",
			Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"op"}),
		PolicyHits: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "policy_evaluations_total",
			Help:      "Count of policy evaluations",
		}, []string{"result"}),
		PoolSelectorResolutions: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "pool_selector_resolutions_total",
			Help:      "Count of metadata-based pool selector attempts",
		}, []string{"tenant", "scope", "vlan", "interface", "ssid", "location", "result"}),
		PoolSelectorErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "pool_selector_errors_total",
			Help:      "Count of pool selector attempts that failed to resolve",
		}, []string{"tenant", "reason"}),
		PoolSelectorLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "pool_selector_resolution_seconds",
			Help:      "Latency of metadata-based pool selector resolutions",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
		}, []string{"tenant", "result"}),
		PoolMetadataSnapshot: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pool_metadata_snapshot",
			Help:      "Latest metadata tuple for pools participating in selector resolutions",
		}, []string{"tenant", "poolId", "scope", "vlan", "interface", "ssid", "location"}),
		PoolServiceLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "pool_service_operation_seconds",
			Help:      "Latency of pool service operations grouped by tenant and operation",
			Buckets:   []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"tenant", "operation", "outcome"}),
		SecurityEvents: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "security_events_total",
			Help:      "Count of security guard events broken down by subsystem",
		}, []string{"type", "result"}),
		SecurityGuardLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "security_guard_latency_seconds",
			Help:      "Time spent inside Guard.Check",
			Buckets:   prometheus.DefBuckets,
		}, []string{"result"}),
		SnoopingCache: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "snooping_cache_events_total",
			Help:      "Count of snooping cache lookups by result",
		}, []string{"result"}),
		DHCPRequestLifecycle: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dhcp_request_lifecycle_total",
			Help:      "Count of DHCP request phases grouped by protocol/message/outcome",
		}, []string{"tenant", "protocol", "message", "outcome"}),
		DHCPRequestLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "dhcp_request_latency_seconds",
			Help:      "End-to-end processing latency for DHCP messages",
			Buckets:   []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}, []string{"tenant", "protocol", "message"}),
		LeaseReplicationLag: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "lease_replication_lag_seconds",
			Help:      "Time between local lease persistence and CDC confirmation",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
		}, []string{"tenant"}),
		LeaseSyncAckFailures: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "lease_sync_ack_failures_total",
			Help:      "Count of lease sync ACK gate failures before DHCP ACK response",
		}, []string{"tenant", "policy", "reason"}),
		SyncTxnReconcileRuns: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "sync_txn_reconcile_total",
			Help:      "Count of cluster sync transaction snapshot reconcile runs",
		}, []string{"result"}),
		LeaseAllocatorLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "lease_allocator_phase_seconds",
			Help:      "Latency of lease allocator phases grouped by tenant and phase",
			Buckets:   []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}, []string{"tenant", "phase", "outcome"}),
		LeaseAllocatorAttempts: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "lease_allocator_phase_total",
			Help:      "Count of lease allocator phase executions grouped by tenant and outcome",
		}, []string{"tenant", "phase", "outcome"}),
		PoolUtilization: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pool_utilization_percent",
			Help:      "Current utilization percentage per address pool",
		}, []string{"tenant", "poolId", "scope"}),
		DBPoolStats: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "db_pool_stats",
			Help:      "Snapshot of database pool statistics grouped by role",
		}, []string{"tenant", "role", "state"}),
		CacheEvents: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_events_total",
			Help:      "Count of cache interactions grouped by layer and outcome",
		}, []string{"resource", "operation", "layer", "result"}),
		MobilityAffinityHits: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "dhcp_mobility_affinity_hit_total",
			Help:      "Count of mobility affinity cache lookups grouped by tenant and result",
		}, []string{"tenant", "result"}),
		HARole: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ha_role",
			Help:      "Current HA role of this node",
		}, []string{"role"}),
		HAState: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ha_state",
			Help:      "Current HA state machine status",
		}, []string{"state"}),
		HAPeerGapSeconds: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ha_peer_lag_seconds",
			Help:      "Seconds since the last heartbeat or replication signal from peer",
		}),
		HAFailoverTransitions: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ha_failover_transitions_total",
			Help:      "Number of HA role transitions",
		}, []string{"from", "to"}),
		HACoordinatorQuorum: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ha_coordinator_quorum",
			Help:      "Coordinator quorum state (type=healthy|total|primary_votes)",
		}, []string{"type"}),
	}
}

// sanitizeNamespace ensures Prometheus namespace strings match the collector regex requirements.
func sanitizeNamespace(ns string) string {
	if ns == "" {
		return ns
	}
	sanitized := namespaceSanitizer.ReplaceAllString(ns, "_")
	if sanitized == "" {
		return sanitized
	}
	if sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "_" + sanitized
	}
	return sanitized
}
