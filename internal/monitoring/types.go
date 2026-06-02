package monitoring

import (
	"time"

	"modern-dhcp/internal/security/snooping"
)

// PoolUsageSummary captures utilization stats for an address pool.
type PoolUsageSummary struct {
	PoolID      string  `json:"poolId"`
	Name        string  `json:"name"`
	Scope       string  `json:"scope"`
	VLANID      int     `json:"vlanId"`
	Location    string  `json:"location"`
	Allocated   int64   `json:"allocated"`
	Capacity    int64   `json:"capacity"`
	Utilization float64 `json:"utilization"`
}

// RequestPhaseSnapshot summarizes DHCP request lifecycle metrics.
type RequestPhaseSnapshot struct {
	Protocol       string          `json:"protocol"`
	Message        string          `json:"message"`
	Success        uint64          `json:"success"`
	Failure        uint64          `json:"failure"`
	AverageMs      float64         `json:"averageMs"`
	P95Ms          float64         `json:"p95Ms"`
	LatencyBuckets []LatencyBucket `json:"latencyBuckets"`
}

// LatencyBucket exposes histogram counts.
type LatencyBucket struct {
	UpperBoundMs float64 `json:"upperBoundMs"`
	Count        uint64  `json:"count"`
}

// DimensionCount is used for heatmap style payloads.
type DimensionCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// ClientDistributionSnapshot surfaces device/location/VLAN groupings.
type ClientDistributionSnapshot struct {
	ByDeviceType []DimensionCount `json:"byDeviceType"`
	ByLocation   []DimensionCount `json:"byLocation"`
	ByVLAN       []DimensionCount `json:"byVlan"`
}

// SystemHealthSnapshot contains runtime + OS level statistics.
type SystemHealthSnapshot struct {
	Timestamp        time.Time `json:"timestamp"`
	CPUPercent       float64   `json:"cpuPercent"`
	MemoryPercent    float64   `json:"memoryPercent"`
	MemoryUsedBytes  uint64    `json:"memoryUsedBytes"`
	DiskPercent      float64   `json:"diskPercent"`
	CPUCores         int       `json:"cpuCores"`
	MemoryTotalBytes uint64    `json:"memoryTotalBytes"`
	DiskUsedBytes    uint64    `json:"diskUsedBytes"`
	DiskTotalBytes   uint64    `json:"diskTotalBytes"`
	NetworkRxBytes   uint64    `json:"networkRxBytes"`
	NetworkTxBytes   uint64    `json:"networkTxBytes"`
	Goroutines       int       `json:"goroutines"`
}

// SecuritySnapshot exposes guard-level telemetry for dashboards.
type SecuritySnapshot struct {
	RateLimit RateLimitWindow `json:"rateLimit"`
	Snooping  SnoopingWindow  `json:"snooping"`
}

// RateLimitEvent captures individual guard rejections for rate limiting.
type RateLimitEvent struct {
	TenantID   string        `json:"tenantId"`
	OccurredAt time.Time     `json:"occurredAt"`
	MAC        string        `json:"mac"`
	PortID     string        `json:"portId"`
	IP         string        `json:"ip"`
	RetryAfter time.Duration `json:"retryAfter"`
}

// SnoopingEvent captures individual snooping observations.
type SnoopingEvent struct {
	TenantID   string                     `json:"tenantId"`
	OccurredAt time.Time                  `json:"occurredAt"`
	MAC        string                     `json:"mac"`
	PortID     string                     `json:"portId"`
	VLANID     int                        `json:"vlanId"`
	Result     snooping.ObservationResult `json:"result"`
	Reason     string                     `json:"reason,omitempty"`
}

// SecurityEventsSnapshot aggregates recent guard events for dashboards.
type SecurityEventsSnapshot struct {
	TenantID    string           `json:"tenantId"`
	GeneratedAt time.Time        `json:"generatedAt"`
	Window      time.Duration    `json:"window"`
	RateLimit   []RateLimitEvent `json:"rateLimit"`
	Snooping    []SnoopingEvent  `json:"snooping"`
}

// OverviewSnapshot collates the dashboard cards.
type OverviewSnapshot struct {
	GeneratedAt        time.Time                  `json:"generatedAt"`
	PoolUsage          []PoolUsageSummary         `json:"poolUsage"`
	RequestPhases      []RequestPhaseSnapshot     `json:"requestPhases"`
	ClientDistribution ClientDistributionSnapshot `json:"clientDistribution"`
	SystemHealth       SystemHealthSnapshot       `json:"systemHealth"`
	Security           SecuritySnapshot           `json:"security"`
}

// CapacityInsight describes pool growth posture for planning dashboards.
type CapacityInsight struct {
	PoolID              string    `json:"poolId"`
	Name                string    `json:"name"`
	Utilization         float64   `json:"utilization"`
	Headroom            int64     `json:"headroom"`
	ProjectedExhaustion time.Time `json:"projectedExhaustion,omitempty"`
	Recommendation      string    `json:"recommendation"`
}

// AnomalyInsight surfaces heuristics-based anomaly detection results.
type AnomalyInsight struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Metric     string    `json:"metric"`
	Severity   string    `json:"severity"`
	Current    float64   `json:"current"`
	Baseline   float64   `json:"baseline"`
	Summary    string    `json:"summary"`
	DetectedAt time.Time `json:"detectedAt"`
}

// AnalyticsSnapshot groups anomaly + capacity insights for advanced dashboards.
type AnalyticsSnapshot struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Capacity    []CapacityInsight `json:"capacity"`
	Anomalies   []AnomalyInsight  `json:"anomalies"`
}

// CMDBSyncPayload packages monitoring insights for CMDB/ITSM ingestion flows.
type CMDBSyncPayload struct {
	TenantID         string             `json:"tenantId"`
	Source           string             `json:"source"`
	GeneratedAt      time.Time          `json:"generatedAt"`
	PoolHotspots     []PoolUsageSummary `json:"poolHotspots"`
	CapacityInsights []CapacityInsight  `json:"capacityInsights"`
	AnomalyInsights  []AnomalyInsight   `json:"anomalyInsights"`
	AlertTotals      AlertFeedTotals    `json:"alertTotals"`
}
