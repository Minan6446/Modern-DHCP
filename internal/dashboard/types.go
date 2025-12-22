package dashboard

import (
	"context"
	"time"

	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/monitoring"
)

// HealthHook mirrors the server health interface so dashboard snapshots can run probes.
type HealthHook interface {
	Name() string
	Check(ctx context.Context) error
}

// CheckStatus captures the outcome of an individual subsystem probe.
type CheckStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// HealthSummary aggregates runtime statistics and probe outcomes for the control plane dashboard.
type HealthSummary struct {
	GeneratedAt time.Time                       `json:"generatedAt"`
	Status      string                          `json:"status"`
	System      monitoring.SystemHealthSnapshot `json:"system"`
	Cluster     *failover.StatusSnapshot        `json:"cluster,omitempty"`
	Checks      []CheckStatus                   `json:"checks"`
}

// KPISnapshot exposes high-level utilization and request success metrics.
type KPISnapshot struct {
	GeneratedAt        time.Time                             `json:"generatedAt"`
	TenantID           string                                `json:"tenantId"`
	ActiveLeases       int64                                 `json:"activeLeases"`
	RequestVolume24h   uint64                                `json:"requestVolume24h"`
	RequestSuccessRate float64                               `json:"requestSuccessRate"`
	TopPools           []monitoring.PoolUsageSummary         `json:"topPools"`
	ClientDistribution monitoring.ClientDistributionSnapshot `json:"clientDistribution"`
	SystemHealth       monitoring.SystemHealthSnapshot       `json:"systemHealth"`
	Security           monitoring.SecuritySnapshot           `json:"security"`
}

// StreamType enumerates supported activity stream categories.
type StreamType string

const (
	// StreamTypeAlert represents alert or event feed entries.
	StreamTypeAlert StreamType = "alert"
	// StreamTypeOperation represents administrative/audit events.
	StreamTypeOperation StreamType = "operation"
)

// StreamEntry unifies alerts, audit activity, and future event bus payloads.
type StreamEntry struct {
	ID         string            `json:"id"`
	Type       StreamType        `json:"type"`
	Severity   string            `json:"severity,omitempty"`
	Summary    string            `json:"summary"`
	Source     string            `json:"source,omitempty"`
	OccurredAt time.Time         `json:"occurredAt"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// StreamSnapshot is returned by /dashboard/streams.
type StreamSnapshot struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	TenantID    string        `json:"tenantId"`
	Items       []StreamEntry `json:"items"`
}

// StreamOptions controls how dashboard stream payloads are assembled.
type StreamOptions struct {
	TenantID          string
	Limit             int
	IncludeAlerts     bool
	IncludeOperations bool
	Since             time.Time
}

// InsightSnapshot mirrors the legacy insights overview payload used by the UI.
type InsightSnapshot struct {
	GeneratedAt     time.Time                  `json:"generatedAt"`
	TenantActivity  TenantActivitySnapshot     `json:"tenantActivity"`
	AlertProcessing AlertProcessingSnapshot    `json:"alertProcessing"`
	Automation      AutomationProgressSnapshot `json:"automation"`
}

// TenantActivitySnapshot summarizes active lease trends and hotspots.
type TenantActivitySnapshot struct {
	TotalActive  int64           `json:"totalActive"`
	DeltaPercent float64         `json:"deltaPercent"`
	Hotspots     []TenantHotspot `json:"hotspots"`
}

// TenantHotspot highlights pools with outsized utilization.
type TenantHotspot struct {
	TenantID     string `json:"tenantId,omitempty"`
	Name         string `json:"name"`
	ActiveLeases int64  `json:"activeLeases"`
	Trend        string `json:"trend"`
}

// AlertProcessingSnapshot captures workflow metrics for alerts.
type AlertProcessingSnapshot struct {
	Open          int     `json:"open"`
	Acknowledged  int     `json:"acknowledged"`
	Suppressed    int     `json:"suppressed"`
	MTTRMinutes   float64 `json:"mttrMinutes"`
	ResponseTrend int     `json:"responseTrend"`
}

// AutomationProgressSnapshot outlines recent automation workflow health.
type AutomationProgressSnapshot struct {
	Completed  int                          `json:"completed"`
	Running    int                          `json:"running"`
	Queued     int                          `json:"queued"`
	Failed     int                          `json:"failed"`
	NextWindow string                       `json:"nextWindow,omitempty"`
	Workflows  []AutomationWorkflowSnapshot `json:"workflows"`
}

// AutomationWorkflowSnapshot describes a single workflow card in the UI.
type AutomationWorkflowSnapshot struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ProgressPercent int    `json:"progressPercent"`
	State           string `json:"state"`
	Owner           string `json:"owner,omitempty"`
	Schedule        string `json:"schedule,omitempty"`
}
