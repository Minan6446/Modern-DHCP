package reporting

import (
	"time"

	"modern-dhcp/internal/monitoring"
	"modern-dhcp/pkg/models"
)

// MonthlyUsageReport summarizes address consumption and client mix.
type MonthlyUsageReport struct {
	TenantID           string                                `json:"tenantId"`
	GeneratedAt        time.Time                             `json:"generatedAt"`
	PoolUsage          []monitoring.PoolUsageSummary         `json:"poolUsage"`
	ClientDistribution monitoring.ClientDistributionSnapshot `json:"clientDistribution"`
	RequestPhases      []monitoring.RequestPhaseSnapshot     `json:"requestPhases"`
	Security           monitoring.SecuritySnapshot           `json:"security"`
	Notes              []string                              `json:"notes,omitempty"`
}

// SecurityComplianceReport consolidates guard findings and admin activity.
type SecurityComplianceReport struct {
	TenantID        string                    `json:"tenantId"`
	GeneratedAt     time.Time                 `json:"generatedAt"`
	Alerts          []SecurityIncident        `json:"alerts"`
	Violations      []SecurityIncident        `json:"violations"`
	AdminActivities []AdminActivityRecord     `json:"adminActivities"`
	Summary         SecurityComplianceSummary `json:"summary"`
}

// SecurityIncident represents a single guard/security audit event.
type SecurityIncident struct {
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"createdAt"`
	Payload   any       `json:"payload,omitempty"`
}

// AdminActivityRecord captures privileged API usage.
type AdminActivityRecord struct {
	Actor       string    `json:"actor"`
	Role        string    `json:"role"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	StatusCode  int       `json:"statusCode"`
	Sensitive   bool      `json:"sensitive"`
	ObservedAt  time.Time `json:"observedAt"`
	Correlation string    `json:"correlationId,omitempty"`
}

// SecurityComplianceSummary aggregates KPI style counts.
type SecurityComplianceSummary struct {
	TotalAlerts     int        `json:"totalAlerts"`
	TotalViolations int        `json:"totalViolations"`
	LastViolationAt *time.Time `json:"lastViolationAt,omitempty"`
}

// CapacityPlanningReport highlights saturation risks.
type CapacityPlanningReport struct {
	TenantID        string                        `json:"tenantId"`
	GeneratedAt     time.Time                     `json:"generatedAt"`
	HotPools        []monitoring.PoolUsageSummary `json:"hotPools"`
	SaturatedPools  []monitoring.PoolUsageSummary `json:"saturatedPools"`
	Recommendations []string                      `json:"recommendations"`
}

// AuditTrailReport provides a timeline for a resource/correlation.
type AuditTrailReport struct {
	TenantID  string               `json:"tenantId"`
	Resource  string               `json:"resource,omitempty"`
	Generated time.Time            `json:"generatedAt"`
	Events    []AuditTimelineEntry `json:"events"`
}

// AuditTimelineEntry expands persisted audit events with decoded payloads.
type AuditTimelineEntry struct {
	AuditID       string            `json:"auditId"`
	Action        string            `json:"action"`
	Actor         string            `json:"actor"`
	Resource      string            `json:"resource"`
	CorrelationID string            `json:"correlationId,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	Payload       map[string]any    `json:"payload,omitempty"`
	Raw           models.AuditEvent `json:"-"`
}
