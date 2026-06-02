package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/audit"
	"modern-dhcp/internal/automation"
	"modern-dhcp/internal/failover"
	"modern-dhcp/internal/lease"
	"modern-dhcp/internal/monitoring"
	"modern-dhcp/internal/scopeutil"
	"modern-dhcp/pkg/auditpayload"
)

var (
	errMonitoringUnavailable = errors.New("dashboard: monitoring aggregator unavailable")
	errAlertFeedUnavailable  = errors.New("dashboard: alert feed unavailable")
)

// Options wires dependencies for the dashboard service.
type Options struct {
	Aggregator   *monitoring.Aggregator
	AlertFeed    *monitoring.AlertFeed
	Automation   *automation.Service
	Audit        *audit.Service
	Coordinator  failover.StatusReporter
	HealthHooks  []HealthHook
	Logger       *zap.Logger
	CacheTTL     time.Duration
	StreamLimit  int
	StreamMax    int
	HotspotLimit int
	Now          func() time.Time
}

// Service aggregates dashboard payloads for the HTTP API.
type Service struct {
	monitor     *monitoring.Aggregator
	alertFeed   *monitoring.AlertFeed
	automation  *automation.Service
	audit       *audit.Service
	coordinator failover.StatusReporter
	hooks       []HealthHook
	logger      *zap.Logger

	cacheTTL     time.Duration
	streamLimit  int
	streamMax    int
	hotspotLimit int
	now          func() time.Time

	cacheMu     sync.RWMutex
	healthCache healthCacheEntry
	kpiCache    map[string]kpiCacheEntry

	insightMu       sync.Mutex
	leaseActivity   map[string]int64
	alertOpenCounts map[string]int
}

// NewService constructs a dashboard service with sane defaults.
func NewService(opts Options) *Service {
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Second
	}
	streamLimit := opts.StreamLimit
	if streamLimit <= 0 {
		streamLimit = 50
	}
	streamMax := opts.StreamMax
	if streamMax <= 0 {
		streamMax = 250
	}
	hotspotLimit := opts.HotspotLimit
	if hotspotLimit <= 0 {
		hotspotLimit = 5
	}
	clock := opts.Now
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		monitor:         opts.Aggregator,
		alertFeed:       opts.AlertFeed,
		automation:      opts.Automation,
		audit:           opts.Audit,
		coordinator:     opts.Coordinator,
		hooks:           append([]HealthHook(nil), opts.HealthHooks...),
		logger:          opts.Logger,
		cacheTTL:        cacheTTL,
		streamLimit:     streamLimit,
		streamMax:       streamMax,
		hotspotLimit:    hotspotLimit,
		now:             clock,
		kpiCache:        make(map[string]kpiCacheEntry),
		leaseActivity:   make(map[string]int64),
		alertOpenCounts: make(map[string]int),
	}
}

// Health returns cached system health with probe results.
func (s *Service) Health(ctx context.Context) (HealthSummary, error) {
	if summary, ok := s.cachedHealth(); ok {
		return summary, nil
	}
	summary := HealthSummary{
		GeneratedAt: s.now(),
		System:      s.snapshotSystem(ctx),
	}
	summary.Checks = s.evaluateHooks(ctx)
	summary.Status = aggregateStatus(summary.Checks)
	if s.coordinator != nil {
		snapshot := s.coordinator.Snapshot()
		summary.Cluster = &snapshot
	}
	s.storeHealth(summary)
	return summary, nil
}

// KPIs exposes summarized utilization metrics for a tenant scope.
func (s *Service) KPIs(ctx context.Context, scope lease.ResourceScope, limit int) (KPISnapshot, error) {
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return KPISnapshot{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	if snapshot, ok := s.cachedKPI(tenantID); ok {
		return snapshot, nil
	}
	if s.monitor == nil {
		return KPISnapshot{}, errMonitoringUnavailable
	}
	if limit <= 0 {
		limit = s.hotspotLimit
	}
	overview, err := s.monitor.Overview(ctx, scope, limit)
	if err != nil {
		return KPISnapshot{}, err
	}
	var (
		activeLeases int64
		totalSuccess uint64
		totalFailure uint64
	)
	for _, pool := range overview.PoolUsage {
		activeLeases += pool.Allocated
	}
	for _, phase := range overview.RequestPhases {
		totalSuccess += phase.Success
		totalFailure += phase.Failure
	}
	totalRequests := totalSuccess + totalFailure
	successRate := 0.0
	if totalRequests > 0 {
		successRate = (float64(totalSuccess) / float64(totalRequests)) * 100
		successRate = math.Round(successRate*10) / 10
	}
	snapshot := KPISnapshot{
		GeneratedAt:        s.now(),
		TenantID:           tenantID,
		ActiveLeases:       activeLeases,
		RequestVolume24h:   totalRequests,
		RequestSuccessRate: successRate,
		TopPools:           overview.PoolUsage,
		ClientDistribution: overview.ClientDistribution,
		SystemHealth:       overview.SystemHealth,
		Security:           overview.Security,
		Scope:              scopeutil.FromLeaseScope(scope),
	}
	s.storeKPI(tenantID, snapshot)
	return snapshot, nil
}

// Streams composes alert, event, and audit activity for dashboards.

func (s *Service) Streams(ctx context.Context, opts StreamOptions) (StreamSnapshot, error) {
	tenantID := strings.TrimSpace(opts.TenantID)
	if tenantID == "" && !opts.Scope.IsZero() {
		tenantID = strings.TrimSpace(opts.Scope.TenantOrDefault())
	}
	if tenantID == "" {
		return StreamSnapshot{}, errors.New("dashboard: tenant scope required")
	}
	scopeMeta := scopeutil.FromLeaseScope(opts.Scope)
	limit := s.normalizeLimit(opts.Limit)
	includeAlerts := opts.IncludeAlerts
	includeOps := opts.IncludeOperations
	if !includeAlerts && !includeOps {
		includeAlerts = true
		includeOps = true
	}
	entries := make([]StreamEntry, 0, limit*2)
	if includeAlerts {
		alerts, err := s.buildAlertStream(tenantID, limit, opts.Since)
		if err != nil && s.logger != nil {
			s.logger.Warn("dashboard: alert stream degraded", zap.String("tenantId", tenantID), zap.Error(err))
		}
		entries = append(entries, alerts...)
	}
	if includeOps {
		opsEntries, err := s.buildOperationStream(ctx, tenantID, limit, opts.Since)
		if err != nil && s.logger != nil {
			s.logger.Warn("dashboard: audit stream degraded", zap.String("tenantId", tenantID), zap.Error(err))
		} else {
			entries = append(entries, opsEntries...)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].OccurredAt.After(entries[j].OccurredAt)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return StreamSnapshot{
		GeneratedAt: s.now(),
		TenantID:    tenantID,
		Items:       entries,
		Scope:       scopeMeta,
	}, nil
}

// Insights returns automation and alert processing summaries for a tenant scope.
func (s *Service) Insights(ctx context.Context, scope lease.ResourceScope) (InsightSnapshot, error) {
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return InsightSnapshot{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	snapshot, buildErr := s.buildInsightSnapshot(ctx, scope)
	if buildErr != nil {
		if s.logger != nil {
			s.logger.Warn("dashboard: insights degraded", zap.String("tenantId", tenantID), zap.Error(buildErr))
		}
		return s.sampleInsightSnapshot(tenantID, scopeutil.FromLeaseScope(scope)), buildErr
	}
	snapshot.Scope = scopeutil.FromLeaseScope(scope)
	return snapshot, nil
}

func (s *Service) evaluateHooks(ctx context.Context) []CheckStatus {
	if len(s.hooks) == 0 {
		return nil
	}
	results := make([]CheckStatus, 0, len(s.hooks))
	deadline := s.cacheTTL
	if deadline > 3*time.Second {
		deadline = 3 * time.Second
	}
	for _, hook := range s.hooks {
		if hook == nil {
			continue
		}
		status := CheckStatus{Name: hook.Name(), Status: "ok"}
		hookCtx, cancel := context.WithTimeout(ctx, deadline)
		if err := hook.Check(hookCtx); err != nil {
			status.Status = "error"
			status.Detail = err.Error()
		}
		cancel()
		results = append(results, status)
	}
	return results
}

func aggregateStatus(checks []CheckStatus) string {
	for _, check := range checks {
		if check.Status != "ok" {
			return "degraded"
		}
	}
	return "ok"
}

func (s *Service) snapshotSystem(ctx context.Context) monitoring.SystemHealthSnapshot {
	if s.monitor == nil {
		return monitoring.SystemHealthSnapshot{Timestamp: s.now()}
	}
	return s.monitor.Health(ctx)
}

func (s *Service) cachedHealth() (HealthSummary, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if s.healthCache.expiry.IsZero() || time.Now().After(s.healthCache.expiry) {
		return HealthSummary{}, false
	}
	return s.healthCache.summary, true
}

func (s *Service) storeHealth(summary HealthSummary) {
	s.cacheMu.Lock()
	s.healthCache = healthCacheEntry{summary: summary, expiry: time.Now().Add(s.cacheTTL)}
	s.cacheMu.Unlock()
}

func (s *Service) cachedKPI(tenantID string) (KPISnapshot, bool) {
	s.cacheMu.RLock()
	entry, ok := s.kpiCache[tenantID]
	s.cacheMu.RUnlock()
	if !ok || entry.expiry.IsZero() || time.Now().After(entry.expiry) {
		return KPISnapshot{}, false
	}
	return entry.snapshot, true
}

func (s *Service) storeKPI(tenantID string, snapshot KPISnapshot) {
	s.cacheMu.Lock()
	s.kpiCache[tenantID] = kpiCacheEntry{snapshot: snapshot, expiry: time.Now().Add(s.cacheTTL)}
	s.cacheMu.Unlock()
}

func (s *Service) normalizeLimit(limit int) int {
	if limit <= 0 {
		limit = s.streamLimit
	}
	if s.streamMax > 0 && limit > s.streamMax {
		return s.streamMax
	}
	return limit
}

func (s *Service) buildAlertStream(tenantID string, limit int, since time.Time) ([]StreamEntry, error) {
	if s.alertFeed == nil {
		return nil, errAlertFeedUnavailable
	}
	snapshot := s.alertFeed.Snapshot(tenantID, limit*2)
	entries := make([]StreamEntry, 0, len(snapshot.Alerts))
	for _, alert := range snapshot.Alerts {
		if !since.IsZero() && alert.CreatedAt.Before(since) {
			continue
		}
		metadata := map[string]string{
			"category":  alert.Category,
			"lifecycle": string(alert.Lifecycle),
		}
		if alert.TenantID != "" {
			metadata["tenantId"] = alert.TenantID
		}
		if alert.Assignee != "" {
			metadata["assignee"] = alert.Assignee
		}
		if len(alert.Tags) > 0 {
			metadata["tags"] = strings.Join(alert.Tags, ",")
		}
		entries = append(entries, StreamEntry{
			ID:         alert.ID,
			Type:       StreamTypeAlert,
			Severity:   string(alert.Severity),
			Summary:    alert.Summary,
			Source:     alert.Source,
			OccurredAt: alert.CreatedAt,
			Metadata:   metadata,
		})
	}
	return entries, nil
}

func (s *Service) buildOperationStream(ctx context.Context, tenantID string, limit int, since time.Time) ([]StreamEntry, error) {
	if s.audit == nil {
		return nil, nil
	}
	events, err := s.audit.ListEvents(ctx, tenantID, limit, 0)
	if err != nil {
		return nil, err
	}
	entries := make([]StreamEntry, 0, len(events))
	for _, evt := range events {
		if !since.IsZero() && evt.CreatedAt.Before(since) {
			continue
		}
		metadata := map[string]string{
			"action":   evt.Action,
			"resource": evt.Resource,
		}
		if evt.Source != "" {
			metadata["source"] = evt.Source
		}
		if evt.CorrelationID != "" {
			metadata["correlationId"] = evt.CorrelationID
		}
		for key, val := range convertMetadata(evt.Payload) {
			metadata[key] = val
		}
		severity := classifyAuditSeverity(evt.Action)
		summary := fmt.Sprintf("%s %s", evt.Actor, evt.Action)
		entries = append(entries, StreamEntry{
			ID:         evt.AuditID,
			Type:       StreamTypeOperation,
			Severity:   severity,
			Summary:    summary,
			Source:     evt.Resource,
			OccurredAt: evt.CreatedAt,
			Metadata:   metadata,
		})
	}
	return entries, nil
}

func convertMetadata(payload []byte) map[string]string {
	if len(payload) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		out[key] = fmt.Sprint(value)
	}
	return out
}

func classifyAuditSeverity(action string) string {
	value := strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.Contains(value, "fail"), strings.Contains(value, "error"), strings.Contains(value, "deny"):
		return "error"
	case strings.Contains(value, "warn"), strings.Contains(value, "pending"):
		return "warning"
	default:
		return "info"
	}
}

func (s *Service) buildInsightSnapshot(ctx context.Context, scope lease.ResourceScope) (InsightSnapshot, error) {
	if s.monitor == nil || s.alertFeed == nil {
		return InsightSnapshot{}, errMonitoringUnavailable
	}
	tenantID, err := scope.TenantIDOrErr()
	if err != nil {
		return InsightSnapshot{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	const poolSample = 25
	scopeMeta := scopeutil.FromLeaseScope(scope)
	pools, err := s.monitor.Pools(ctx, scope, poolSample)
	if err != nil {
		return InsightSnapshot{}, err
	}
	totalActive := int64(0)
	hotspotLimit := minInt(s.hotspotLimit, len(pools))
	hotspots := make([]TenantHotspot, 0, hotspotLimit)
	for idx, pool := range pools {
		totalActive += pool.Allocated
		if idx < hotspotLimit {
			hotspots = append(hotspots, TenantHotspot{
				TenantID:     tenantID,
				Name:         pool.Name,
				ActiveLeases: pool.Allocated,
				Trend:        poolTrend(pool.Utilization),
			})
		}
	}
	delta := s.trackLeaseDelta(tenantID, totalActive)
	alerts := s.alertFeed.Snapshot(tenantID, 25)
	trend := s.trackAlertTrend(tenantID, alerts.Totals.Open)
	mttr := calcMTTR(alerts.Alerts)
	automation := s.buildAutomationSnapshot(tenantID, scope)
	snapshot := InsightSnapshot{
		GeneratedAt: s.now(),
		TenantActivity: TenantActivitySnapshot{
			TotalActive:  totalActive,
			DeltaPercent: delta,
			Hotspots:     hotspots,
		},
		AlertProcessing: AlertProcessingSnapshot{
			Open:          alerts.Totals.Open,
			Acknowledged:  alerts.Totals.Acknowledged,
			Suppressed:    alerts.Totals.Suppressed,
			MTTRMinutes:   mttr,
			ResponseTrend: trend,
		},
		Automation: automation,
	}
	snapshot.Scope = scopeMeta
	return snapshot, nil
}

func (s *Service) buildAutomationSnapshot(tenantID string, scope lease.ResourceScope) AutomationProgressSnapshot {
	snapshot := AutomationProgressSnapshot{
		Workflows:  make([]AutomationWorkflowSnapshot, 0, 5),
		NextWindow: s.now().Add(30 * time.Minute).Format(time.RFC3339),
	}
	phases := []monitoring.RequestPhaseSnapshot(nil)
	if s.monitor != nil {
		phases = s.monitor.Requests(scope)
	}
	for idx, phase := range phases {
		if len(snapshot.Workflows) >= 5 {
			break
		}
		state := workflowState(phase.Success, phase.Failure)
		progress := workflowProgress(phase.Success, phase.Failure)
		interval := clampInt(int(math.Round(phase.P95Ms/25)), 1, 120)
		workflow := AutomationWorkflowSnapshot{
			ID:              fmt.Sprintf("wf-%d", idx),
			Name:            fmt.Sprintf("%s %s automation", phase.Protocol, strings.ToUpper(phase.Message)),
			ProgressPercent: progress,
			State:           state,
			Owner:           strings.ToUpper(phase.Protocol) + " pipeline",
			Schedule:        fmt.Sprintf("every %dm", interval),
		}
		snapshot.Workflows = append(snapshot.Workflows, workflow)
		switch state {
		case workflowStateBlocked:
			snapshot.Failed++
		case workflowStateWarn:
			snapshot.Running++
		default:
			snapshot.Completed++
		}
	}
	processed := snapshot.Completed + snapshot.Running + snapshot.Failed
	if total := len(snapshot.Workflows); total > processed {
		snapshot.Queued = total - processed
	}
	return snapshot
}

func (s *Service) trackLeaseDelta(tenantID string, current int64) float64 {
	s.insightMu.Lock()
	defer s.insightMu.Unlock()
	previous := s.leaseActivity[tenantID]
	s.leaseActivity[tenantID] = current
	if previous == 0 {
		if current == 0 {
			return 0
		}
		return 100
	}
	delta := (float64(current-previous) / float64(previous)) * 100
	return math.Round(delta*10) / 10
}

func (s *Service) trackAlertTrend(tenantID string, open int) int {
	s.insightMu.Lock()
	defer s.insightMu.Unlock()
	previous := s.alertOpenCounts[tenantID]
	s.alertOpenCounts[tenantID] = open
	return open - previous
}

func calcMTTR(entries []monitoring.AlertFeedEntry) float64 {
	if len(entries) == 0 {
		return 0
	}
	var total time.Duration
	count := 0
	for _, entry := range entries {
		if entry.UpdatedAt.IsZero() || !entry.UpdatedAt.After(entry.CreatedAt) {
			continue
		}
		total += entry.UpdatedAt.Sub(entry.CreatedAt)
		count++
	}
	if count == 0 || total <= 0 {
		return 0
	}
	minutes := total.Minutes() / float64(count)
	return math.Round(minutes*10) / 10
}

func workflowProgress(success, failure uint64) int {
	total := success + failure
	if total == 0 {
		return 0
	}
	percent := (float64(success) / float64(total)) * 100
	return clampInt(int(math.Round(percent)), 0, 100)
}

func workflowState(success, failure uint64) string {
	total := success + failure
	if total == 0 {
		return workflowStateOK
	}
	rate := float64(failure) / float64(total)
	switch {
	case rate >= 0.3:
		return workflowStateBlocked
	case rate >= 0.1:
		return workflowStateWarn
	default:
		return workflowStateOK
	}
}

func poolTrend(util float64) string {
	switch {
	case util >= 80:
		return "up"
	case util <= 35:
		return "down"
	default:
		return "flat"
	}
}

func clampInt(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

const (
	workflowStateOK      = "ok"
	workflowStateWarn    = "warn"
	workflowStateBlocked = "blocked"
)

func (s *Service) sampleInsightSnapshot(tenantID string, scopeMeta auditpayload.ScopeMetadata) InsightSnapshot {
	now := s.now()
	alerts := monitoring.SampleAlertEntries(tenantID)
	totals := sampleAlertTotals(alerts)
	hotspots := []TenantHotspot{
		{TenantID: tenantID, Name: "hq-prod-vlan12", ActiveLeases: 18450, Trend: "up"},
		{TenantID: tenantID, Name: "edge-relay-05", ActiveLeases: 14320, Trend: "flat"},
		{TenantID: tenantID, Name: "lab-v6-fleet", ActiveLeases: 8200, Trend: "down"},
	}
	automation := AutomationProgressSnapshot{
		Workflows: []AutomationWorkflowSnapshot{
			{ID: "wf-sample-lease-reclaim", Name: "Lease reclaim sweep", ProgressPercent: 92, State: workflowStateOK, Owner: "lease-automation", Schedule: "every 15m"},
			{ID: "wf-sample-guardrails", Name: "Guardrail approvals", ProgressPercent: 64, State: workflowStateWarn, Owner: "policy-engine", Schedule: "every 30m"},
			{ID: "wf-sample-v6-rotation", Name: "IPv6 prefix rotation", ProgressPercent: 47, State: workflowStateOK, Owner: "dhcpv6-core", Schedule: "hourly"},
			{ID: "wf-sample-edge-drain", Name: "Edge relay drain", ProgressPercent: 21, State: workflowStateBlocked, Owner: "failover", Schedule: "manual"},
		},
		NextWindow: now.Add(20 * time.Minute).Format(time.RFC3339),
	}
	for _, wf := range automation.Workflows {
		switch wf.State {
		case workflowStateBlocked:
			automation.Failed++
		case workflowStateWarn:
			automation.Running++
		default:
			automation.Completed++
		}
	}
	processed := automation.Completed + automation.Running + automation.Failed
	if total := len(automation.Workflows); total > processed {
		automation.Queued = total - processed
	}
	return InsightSnapshot{
		GeneratedAt: now,
		TenantActivity: TenantActivitySnapshot{
			TotalActive:  48230,
			DeltaPercent: 3.5,
			Hotspots:     hotspots,
		},
		AlertProcessing: AlertProcessingSnapshot{
			Open:          totals.Open,
			Acknowledged:  totals.Acknowledged,
			Suppressed:    totals.Suppressed,
			MTTRMinutes:   12.4,
			ResponseTrend: 2,
		},
		Automation: automation,
		Scope:      scopeMeta,
	}
}

func sampleAlertTotals(entries []monitoring.AlertFeedEntry) monitoring.AlertFeedTotals {
	var totals monitoring.AlertFeedTotals
	for _, entry := range entries {
		switch entry.Lifecycle {
		case monitoring.AlertLifecycleAcknowledged:
			totals.Acknowledged++
		case monitoring.AlertLifecycleSuppressed:
			totals.Suppressed++
		default:
			totals.Open++
		}
	}
	return totals
}

type healthCacheEntry struct {
	summary HealthSummary
	expiry  time.Time
}

type kpiCacheEntry struct {
	snapshot KPISnapshot
	expiry   time.Time
}
