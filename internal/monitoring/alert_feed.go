package monitoring

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"modern-dhcp/internal/alerting"
)

var (
	// ErrAlertNotFound indicates the requested alert is no longer in the feed window.
	ErrAlertNotFound = errors.New("monitoring: alert not found")
)

// AlertLifecycle describes the processing state for a recorded alert.
type AlertLifecycle string

const (
	// AlertLifecycleOpen indicates an alert awaiting action.
	AlertLifecycleOpen AlertLifecycle = "open"
	// AlertLifecycleAcknowledged indicates an alert that has been triaged.
	AlertLifecycleAcknowledged AlertLifecycle = "acknowledged"
	// AlertLifecycleSuppressed indicates an alert that was muted or auto-resolved.
	AlertLifecycleSuppressed AlertLifecycle = "suppressed"
)

// AlertFeedEntry captures details for dashboard consumption.
type AlertFeedEntry struct {
	ID          string            `json:"id"`
	Summary     string            `json:"summary"`
	Details     string            `json:"details,omitempty"`
	Category    string            `json:"category"`
	Severity    alerting.Severity `json:"severity"`
	Lifecycle   AlertLifecycle    `json:"lifecycle"`
	Source      string            `json:"source,omitempty"`
	TenantID    string            `json:"tenantId,omitempty"`
	Assignee    string            `json:"assignee,omitempty"`
	Channel     string            `json:"channel,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Fingerprint string            `json:"fingerprint,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt,omitempty"`
}

// AlertFeedTotals summarizes lifecycle counts for a tenant.
type AlertFeedTotals struct {
	Open         int `json:"open"`
	Acknowledged int `json:"acknowledged"`
	Suppressed   int `json:"suppressed"`
}

// AlertFeedSnapshot exposes alert stream payloads consumed by the UI.
type AlertFeedSnapshot struct {
	GeneratedAt time.Time        `json:"generatedAt"`
	Totals      AlertFeedTotals  `json:"totals"`
	Alerts      []AlertFeedEntry `json:"alerts"`
}

// AlertObserver captures normalized alert activity.
type AlertObserver interface {
	Record(event alerting.Event)
}

// AlertFeed stores a rolling window of alert activity per tenant.
type AlertFeed struct {
	mu        sync.RWMutex
	perTenant map[string][]AlertFeedEntry
	limit     int
}

// NewAlertFeed constructs a feed with the provided history limit.
func NewAlertFeed(limit int) *AlertFeed {
	if limit <= 0 {
		limit = 100
	}
	return &AlertFeed{perTenant: make(map[string][]AlertFeedEntry), limit: limit}
}

// Record stores the incoming alert for later retrieval.
func (f *AlertFeed) Record(event alerting.Event) {
	if f == nil {
		return
	}
	canonical := normalizeTenant(event.TenantID)
	tenantKey := strings.ToLower(canonical)
	entry := newAlertFeedEntry(event)
	entry.TenantID = canonical
	f.mu.Lock()
	defer f.mu.Unlock()
	entries := append([]AlertFeedEntry{entry}, f.perTenant[tenantKey]...)
	if len(entries) > f.limit {
		entries = entries[:f.limit]
	}
	f.perTenant[tenantKey] = entries
}

// Snapshot returns the latest alert feed for a tenant.
func (f *AlertFeed) Snapshot(tenantID string, limit int) AlertFeedSnapshot {
	snapshot := AlertFeedSnapshot{GeneratedAt: time.Now().UTC()}
	if f == nil {
		return snapshot
	}
	tenantID = normalizeTenant(tenantID)
	tenantKey := strings.ToLower(tenantID)
	f.mu.RLock()
	defer f.mu.RUnlock()
	entries := f.perTenant[tenantKey]
	if limit <= 0 || limit > f.limit {
		limit = f.limit
	}
	if limit > len(entries) {
		limit = len(entries)
	}
	if limit > 0 {
		snapshot.Alerts = make([]AlertFeedEntry, limit)
		copy(snapshot.Alerts, entries[:limit])
	}
	snapshot.Totals = aggregateAlertTotals(entries)
	return snapshot
}

// Acknowledge marks an alert as acknowledged and updates metadata.
func (f *AlertFeed) Acknowledge(tenantID, alertID, assignee string) (AlertFeedEntry, error) {
	assignee = strings.TrimSpace(assignee)
	return f.updateEntry(tenantID, alertID, func(entry *AlertFeedEntry) {
		entry.Lifecycle = AlertLifecycleAcknowledged
		if assignee != "" {
			entry.Assignee = assignee
		}
	})
}

// Suppress marks an alert as suppressed and optionally records the channel used.
func (f *AlertFeed) Suppress(tenantID, alertID, channel string) (AlertFeedEntry, error) {
	channel = strings.TrimSpace(channel)
	return f.updateEntry(tenantID, alertID, func(entry *AlertFeedEntry) {
		entry.Lifecycle = AlertLifecycleSuppressed
		if channel != "" {
			entry.Channel = channel
		}
	})
}

func (f *AlertFeed) updateEntry(tenantID, alertID string, mutate func(*AlertFeedEntry)) (AlertFeedEntry, error) {
	if f == nil {
		return AlertFeedEntry{}, ErrAlertNotFound
	}
	tenantID = normalizeTenant(tenantID)
	tenantKey := strings.ToLower(tenantID)
	f.mu.Lock()
	defer f.mu.Unlock()
	entries := f.perTenant[tenantKey]
	for idx := range entries {
		if entries[idx].ID == alertID {
			mutate(&entries[idx])
			entries[idx].UpdatedAt = time.Now().UTC()
			f.perTenant[tenantKey] = entries
			return entries[idx], nil
		}
	}
	return AlertFeedEntry{}, ErrAlertNotFound
}

// Seed pre-populates the feed for a tenant when no historical alerts exist yet.
func (f *AlertFeed) Seed(tenantID string, entries []AlertFeedEntry) {
	if f == nil || len(entries) == 0 {
		return
	}
	tenantID = normalizeTenant(tenantID)
	tenantKey := strings.ToLower(tenantID)
	f.mu.Lock()
	if len(f.perTenant[tenantKey]) > 0 {
		f.mu.Unlock()
		return
	}
	limit := f.limit
	if limit <= 0 {
		limit = len(entries)
	}
	max := limit
	if len(entries) < max {
		max = len(entries)
	}
	seed := make([]AlertFeedEntry, 0, max)
	baseline := time.Now().UTC()
	for idx := 0; idx < max; idx++ {
		entry := entries[idx]
		entry.TenantID = tenantID
		if entry.ID == "" {
			entry.ID = uuid.NewString()
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = baseline.Add(-time.Duration(idx) * time.Minute)
		}
		seed = append(seed, entry)
	}
	f.perTenant[tenantKey] = seed
	f.mu.Unlock()
}

// Totals returns lifecycle counts for a tenant without copying entries.
func (f *AlertFeed) Totals(tenantID string) AlertFeedTotals {
	if f == nil {
		return AlertFeedTotals{}
	}
	tenantID = normalizeTenant(tenantID)
	tenantKey := strings.ToLower(tenantID)
	f.mu.RLock()
	defer f.mu.RUnlock()
	return aggregateAlertTotals(f.perTenant[tenantKey])
}

func newAlertFeedEntry(event alerting.Event) AlertFeedEntry {
	occurred := event.OccurredAt
	if occurred.IsZero() {
		occurred = time.Now().UTC()
	}
	id := strings.TrimSpace(event.ID)
	if id == "" {
		id = uuid.NewString()
	}
	lifecycle := deriveLifecycle(event.Severity)
	entry := AlertFeedEntry{
		ID:          id,
		Summary:     strings.TrimSpace(event.Summary),
		Details:     strings.TrimSpace(event.Details),
		Category:    strings.TrimSpace(event.Category),
		Severity:    event.Severity,
		Lifecycle:   lifecycle,
		TenantID:    normalizeTenant(event.TenantID),
		Source:      strings.ToLower(strings.TrimSpace(event.Category)),
		Fingerprint: alerting.Fingerprint(event),
		CreatedAt:   occurred,
	}
	if len(event.Labels) > 0 {
		if source, ok := event.Labels["source"]; ok {
			entry.Source = strings.TrimSpace(source)
		}
		if assignee, ok := event.Labels["assignee"]; ok {
			entry.Assignee = strings.TrimSpace(assignee)
		}
	}
	if len(event.Resources) > 0 {
		entry.Tags = append(entry.Tags, event.Resources...)
	}
	if lifecycle == AlertLifecycleAcknowledged {
		entry.UpdatedAt = occurred.Add(2 * time.Minute)
	}
	if lifecycle == AlertLifecycleSuppressed {
		entry.UpdatedAt = occurred.Add(5 * time.Minute)
	}
	return entry
}

func deriveLifecycle(sev alerting.Severity) AlertLifecycle {
	switch sev {
	case alerting.SeverityCritical, alerting.SeverityMajor:
		return AlertLifecycleOpen
	case alerting.SeverityWarning:
		return AlertLifecycleAcknowledged
	default:
		return AlertLifecycleSuppressed
	}
}

func aggregateAlertTotals(entries []AlertFeedEntry) AlertFeedTotals {
	var totals AlertFeedTotals
	for _, entry := range entries {
		switch entry.Lifecycle {
		case AlertLifecycleAcknowledged:
			totals.Acknowledged++
		case AlertLifecycleSuppressed:
			totals.Suppressed++
		default:
			totals.Open++
		}
	}
	return totals
}

// SampleAlertEntries returns representative alerts for tenant demo payloads.
func SampleAlertEntries(tenantID string) []AlertFeedEntry {
	tenantID = normalizeTenant(tenantID)
	now := time.Now().UTC()
	entries := []AlertFeedEntry{
		{
			Summary:   "Pool HQ-Prod utilization 92%",
			Details:   "Pool hq-prod-vlan12 allocated 18450/20000 leases",
			Category:  "POOL_UTILIZATION",
			Severity:  alerting.SeverityMajor,
			Lifecycle: AlertLifecycleOpen,
			Source:    "monitoring",
			TenantID:  tenantID,
			Tags:      []string{"pool:hq-prod-vlan12", "scope:prod"},
			CreatedAt: now.Add(-4 * time.Minute),
		},
		{
			Summary:   "Latency spike: DHCPv4 REQUEST p95 820ms",
			Details:   "Rack dc-a1 uplinks saturated; monitor relay path",
			Category:  "REQUEST_LATENCY",
			Severity:  alerting.SeverityCritical,
			Lifecycle: AlertLifecycleOpen,
			Source:    "monitoring",
			TenantID:  tenantID,
			Tags:      []string{"protocol:dhcpv4", "phase:request"},
			CreatedAt: now.Add(-9 * time.Minute),
		},
		{
			Summary:   "Automation workflow pending approval",
			Details:   "Policy rollout blocked awaiting auditor sign-off",
			Category:  "AUTOMATION",
			Severity:  alerting.SeverityWarning,
			Lifecycle: AlertLifecycleAcknowledged,
			Source:    "automation",
			TenantID:  tenantID,
			Assignee:  "ops-rotation",
			Tags:      []string{"workflow:policy-rollout"},
			CreatedAt: now.Add(-22 * time.Minute),
		},
		{
			Summary:   "Anomaly suppressed: Discover burst",
			Details:   "Edge-relay-05 generated discover flood; auto-muted",
			Category:  "ANOMALY_DETECTED",
			Severity:  alerting.SeverityWarning,
			Lifecycle: AlertLifecycleSuppressed,
			Source:    "anomaly",
			TenantID:  tenantID,
			Channel:   "webhook",
			Tags:      []string{"relay:edge-05"},
			CreatedAt: now.Add(-35 * time.Minute),
		},
	}
	return entries
}
