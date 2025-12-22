package monitoring

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
)

// AlertControllerOptions wires config + dependencies for the alert loop.
type AlertControllerOptions struct {
	Config      config.AlertingConfig
	Aggregator  *Aggregator
	RateTracker RateLimitTracker
	Manager     *alerting.Manager
	Observer    AlertObserver
	Logger      *zap.Logger
}

// AlertController evaluates monitoring snapshots and emits alerts.
type AlertController struct {
	cfg          config.AlertingConfig
	aggregator   *Aggregator
	rateTracker  RateLimitTracker
	manager      *alerting.Manager
	observer     AlertObserver
	logger       *zap.Logger
	policies     []policySettings
	silence      map[string]time.Time
	rules        []compiledRule
	ruleCooldown map[string]time.Time
	mu           sync.Mutex
}

// NewAlertController builds an alert evaluation worker.
func NewAlertController(opts AlertControllerOptions) *AlertController {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	policies := resolvePolicies(opts.Config)
	rules := make([]compiledRule, 0, len(opts.Config.Rules))
	for _, ruleCfg := range opts.Config.Rules {
		rule, err := compileRule(ruleCfg)
		if err != nil {
			logger.Warn("alert rule skipped", zap.String("ruleId", ruleCfg.ID), zap.Error(err))
			continue
		}
		rules = append(rules, rule)
	}
	return &AlertController{
		cfg:          opts.Config,
		aggregator:   opts.Aggregator,
		rateTracker:  opts.RateTracker,
		manager:      opts.Manager,
		observer:     opts.Observer,
		logger:       logger,
		policies:     policies,
		silence:      make(map[string]time.Time),
		rules:        rules,
		ruleCooldown: make(map[string]time.Time),
	}
}

// Start runs the periodic evaluation loop until the context is canceled.
func (c *AlertController) Start(ctx context.Context) {
	if c == nil || !c.cfg.Enabled || c.aggregator == nil || c.manager == nil {
		return
	}
	if len(c.policies) == 0 {
		c.logger.Debug("alert controller disabled; no tenant policies")
		return
	}
	interval := c.cfg.EvaluateInterval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.evaluate(ctx)
			}
		}
	}()
}

func (c *AlertController) evaluate(ctx context.Context) {
	for _, policy := range c.policies {
		if !policy.enabled {
			continue
		}
		c.evaluatePools(ctx, policy)
		c.evaluateRequests(ctx, policy)
		c.evaluateRateLimits(ctx, policy)
		c.evaluateRules(ctx, policy)
	}
}

func (c *AlertController) evaluatePools(ctx context.Context, policy policySettings) {
	if policy.poolThresholds == (config.PoolThresholdConfig{}) {
		return
	}
	limit := policy.poolSampleSize
	if limit <= 0 {
		limit = 50
	}
	usage, err := c.aggregator.Pools(ctx, policy.tenantID, limit)
	if err != nil {
		c.logger.Warn("pool snapshot failed", zap.String("tenant", policy.tenantID), zap.Error(err))
		return
	}
	for _, summary := range usage {
		severity, ok := pickSeverity(summary.Utilization, policy.poolThresholds)
		if !ok {
			continue
		}
		event := alerting.Event{
			Severity:   severity,
			TenantID:   policy.tenantID,
			Category:   "POOL_UTILIZATION",
			Summary:    fmt.Sprintf("Pool %s utilization %.1f%%", summary.Name, summary.Utilization),
			Details:    fmt.Sprintf("Pool %s (%s) allocated %d/%d leases (%.1f%%)", summary.Name, summary.Scope, summary.Allocated, summary.Capacity, summary.Utilization),
			Labels:     map[string]string{"poolId": summary.PoolID, "scope": summary.Scope},
			Resources:  []string{summary.PoolID},
			OccurredAt: time.Now().UTC(),
		}
		c.emit(ctx, policy, event)
	}
}

func (c *AlertController) evaluateRequests(ctx context.Context, policy policySettings) {
	snapshots := c.aggregator.Requests(policy.tenantID)
	for _, snapshot := range snapshots {
		if severity, ok := pickLatencySeverity(snapshot.P95Ms, policy.latencyThresholds); ok {
			event := alerting.Event{
				Severity:  severity,
				TenantID:  policy.tenantID,
				Category:  "REQUEST_LATENCY",
				Summary:   fmt.Sprintf("%s %s p95 %.1fms", snapshot.Protocol, snapshot.Message, snapshot.P95Ms),
				Details:   fmt.Sprintf("Phase %s/%s average %.1fms, p95 %.1fms", snapshot.Protocol, snapshot.Message, snapshot.AverageMs, snapshot.P95Ms),
				Labels:    map[string]string{"protocol": snapshot.Protocol, "message": snapshot.Message},
				Resources: []string{routeKey(snapshot.Protocol, snapshot.Message)},
			}
			c.emit(ctx, policy, event)
		}
		if severity, ok := pickErrorSeverity(snapshot.Success, snapshot.Failure, policy.errorThresholds); ok {
			total := snapshot.Success + snapshot.Failure
			rate := percent(snapshot.Failure, total)
			event := alerting.Event{
				Severity:  severity,
				TenantID:  policy.tenantID,
				Category:  "REQUEST_ERROR_RATE",
				Summary:   fmt.Sprintf("%s %s failure %.1f%%", snapshot.Protocol, snapshot.Message, rate),
				Details:   fmt.Sprintf("Phase %s/%s recorded %d failures out of %d requests", snapshot.Protocol, snapshot.Message, snapshot.Failure, total),
				Labels:    map[string]string{"protocol": snapshot.Protocol, "message": snapshot.Message},
				Resources: []string{routeKey(snapshot.Protocol, snapshot.Message)},
			}
			c.emit(ctx, policy, event)
		}
	}
}

func (c *AlertController) evaluateRateLimits(ctx context.Context, policy policySettings) {
	tracker := c.rateTracker
	thresholds := policy.rateLimitThresholds
	if tracker == nil || thresholds.Window <= 0 {
		return
	}
	stats := tracker.Snapshot(policy.tenantID, thresholds.Window)
	if stats.TotalHits == 0 {
		return
	}
	if severity, ok := pickRateLimitSeverity(stats.TotalHits, thresholds); ok {
		windowStr := thresholds.Window.String()
		summary := fmt.Sprintf("%d rate-limit rejections in %s", stats.TotalHits, windowStr)
		details := fmt.Sprintf("Unique MACs %d, ports %d, last MAC %s on %s", stats.UniqueMACs, stats.UniquePorts, stats.LastMAC, stats.LastPort)
		labels := map[string]string{"source": "guard"}
		resources := []string{"security/ratelimit"}
		event := alerting.Event{
			Severity:   severity,
			TenantID:   policy.tenantID,
			Category:   "RATE_LIMIT",
			Summary:    summary,
			Details:    details,
			Labels:     labels,
			Resources:  resources,
			OccurredAt: stats.LastHit,
		}
		c.emit(ctx, policy, event)
	}
}

func (c *AlertController) evaluateRules(ctx context.Context, policy policySettings) {
	if len(c.rules) == 0 || c.aggregator == nil {
		return
	}
	limit := policy.poolSampleSize
	if limit <= 0 {
		limit = 50
	}
	usage, err := c.aggregator.Pools(ctx, policy.tenantID, limit)
	if err != nil {
		c.logger.Warn("rule evaluation pool snapshot failed", zap.String("tenant", policy.tenantID), zap.Error(err))
		return
	}
	requests := c.aggregator.Requests(policy.tenantID)
	security := c.aggregator.Security(policy.tenantID)
	metrics := buildRuleMetrics(usage, requests, security)
	now := time.Now().UTC()
	for _, rule := range c.rules {
		if rule.id == "" || !rule.matches(metrics) {
			continue
		}
		if c.ruleThrottled(policy.tenantID, rule, now) {
			continue
		}
		summary := strings.TrimSpace(renderTemplate(rule.summary, metrics))
		if summary == "" {
			summary = fmt.Sprintf("Rule %s triggered", rule.id)
		}
		detail := strings.TrimSpace(renderTemplate(rule.detail, metrics))
		labels := map[string]string{"ruleId": rule.id}
		event := alerting.Event{
			ID:        rule.id,
			Severity:  rule.severity,
			TenantID:  policy.tenantID,
			Category:  "RULE_MATCH",
			Summary:   summary,
			Details:   detail,
			Labels:    labels,
			Resources: []string{rule.id},
			Channels:  append([]string(nil), rule.channels...),
		}
		c.emit(ctx, policy, event)
	}
}

func (c *AlertController) ruleThrottled(tenantID string, rule compiledRule, now time.Time) bool {
	if rule.id == "" {
		return true
	}
	ttl := rule.ttl
	if ttl <= 0 {
		ttl = c.cfg.DedupeWindow
		if ttl <= 0 {
			ttl = 5 * time.Minute
		}
	}
	key := ruleCooldownKey(tenantID, rule.id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if expiry, ok := c.ruleCooldown[key]; ok && now.Before(expiry) {
		return true
	}
	c.ruleCooldown[key] = now.Add(ttl)
	return false
}

func (c *AlertController) emit(ctx context.Context, policy policySettings, event alerting.Event) {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if policy.silenceWindow > 0 {
		key := silenceKey(policy.tenantID, event.Category, event.Resources)
		c.mu.Lock()
		last := c.silence[key]
		if time.Since(last) < policy.silenceWindow {
			c.mu.Unlock()
			return
		}
		c.silence[key] = time.Now()
		c.mu.Unlock()
	}
	c.manager.Notify(ctx, event)
	if c.observer != nil {
		c.observer.Record(event)
	}
}

func resolvePolicies(cfg config.AlertingConfig) []policySettings {
	settings := make([]policySettings, 0, len(cfg.Policies))
	for tenantID, override := range cfg.Policies {
		settings = append(settings, mergePolicy(cfg.DefaultPolicy, override, tenantID))
	}
	sort.Slice(settings, func(i, j int) bool { return settings[i].tenantID < settings[j].tenantID })
	return settings
}

func mergePolicy(base, override config.AlertPolicyConfig, tenantID string) policySettings {
	result := policySettings{tenantID: tenantID, enabled: true}
	if base.Enabled != nil {
		result.enabled = *base.Enabled
	}
	if override.Enabled != nil {
		result.enabled = *override.Enabled
	}
	result.silenceWindow = selectDuration(override.SilenceWindow, base.SilenceWindow)
	result.poolSampleSize = selectInt(override.PoolSampleSize, base.PoolSampleSize, 50)
	result.poolThresholds = selectPoolThreshold(base.PoolThresholds, override.PoolThresholds)
	result.latencyThresholds = selectLatencyThreshold(base.Latency, override.Latency)
	result.errorThresholds = selectErrorThreshold(base.ErrorRate, override.ErrorRate)
	result.rateLimitThresholds = selectRateLimitThreshold(base.RateLimit, override.RateLimit)
	return result
}

func pickSeverity(util float64, thresholds config.PoolThresholdConfig) (alerting.Severity, bool) {
	switch {
	case thresholds.Critical > 0 && util >= thresholds.Critical:
		return alerting.SeverityCritical, true
	case thresholds.Major > 0 && util >= thresholds.Major:
		return alerting.SeverityMajor, true
	case thresholds.Warning > 0 && util >= thresholds.Warning:
		return alerting.SeverityWarning, true
	default:
		return "", false
	}
}

func pickLatencySeverity(p95 float64, thresholds config.LatencyThresholdConfig) (alerting.Severity, bool) {
	switch {
	case thresholds.Critical > 0 && p95 >= thresholds.Critical:
		return alerting.SeverityCritical, true
	case thresholds.Major > 0 && p95 >= thresholds.Major:
		return alerting.SeverityMajor, true
	default:
		return "", false
	}
}

func pickErrorSeverity(success, failure uint64, thresholds config.ErrorRateThresholdConfig) (alerting.Severity, bool) {
	total := success + failure
	if total == 0 {
		return "", false
	}
	rate := percent(failure, total)
	switch {
	case thresholds.Critical > 0 && rate >= thresholds.Critical:
		return alerting.SeverityCritical, true
	case thresholds.Major > 0 && rate >= thresholds.Major:
		return alerting.SeverityMajor, true
	case thresholds.Warning > 0 && rate >= thresholds.Warning:
		return alerting.SeverityWarning, true
	default:
		return "", false
	}
}

func pickRateLimitSeverity(count int, thresholds config.RateLimitAlertConfig) (alerting.Severity, bool) {
	switch {
	case thresholds.Critical > 0 && count >= thresholds.Critical:
		return alerting.SeverityCritical, true
	case thresholds.Major > 0 && count >= thresholds.Major:
		return alerting.SeverityMajor, true
	case thresholds.Warning > 0 && count >= thresholds.Warning:
		return alerting.SeverityWarning, true
	default:
		return "", false
	}
}

func percent(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return (float64(part) / float64(total)) * 100
}

func selectDuration(primary, fallback time.Duration) time.Duration {
	switch {
	case primary > 0:
		return primary
	case fallback > 0:
		return fallback
	default:
		return 0
	}
}

func selectInt(primary, fallback, defaultVal int) int {
	switch {
	case primary > 0:
		return primary
	case fallback > 0:
		return fallback
	default:
		return defaultVal
	}
}

func selectPoolThreshold(base, override config.PoolThresholdConfig) config.PoolThresholdConfig {
	result := base
	if override.Warning > 0 {
		result.Warning = override.Warning
	}
	if override.Major > 0 {
		result.Major = override.Major
	}
	if override.Critical > 0 {
		result.Critical = override.Critical
	}
	return result
}

func selectLatencyThreshold(base, override config.LatencyThresholdConfig) config.LatencyThresholdConfig {
	result := base
	if override.Major > 0 {
		result.Major = override.Major
	}
	if override.Critical > 0 {
		result.Critical = override.Critical
	}
	return result
}

func selectErrorThreshold(base, override config.ErrorRateThresholdConfig) config.ErrorRateThresholdConfig {
	result := base
	if override.Warning > 0 {
		result.Warning = override.Warning
	}
	if override.Major > 0 {
		result.Major = override.Major
	}
	if override.Critical > 0 {
		result.Critical = override.Critical
	}
	return result
}

func selectRateLimitThreshold(base, override config.RateLimitAlertConfig) config.RateLimitAlertConfig {
	result := base
	if override.Window > 0 {
		result.Window = override.Window
	}
	if override.Warning > 0 {
		result.Warning = override.Warning
	}
	if override.Major > 0 {
		result.Major = override.Major
	}
	if override.Critical > 0 {
		result.Critical = override.Critical
	}
	return result
}

func silenceKey(tenantID, category string, resources []string) string {
	sorted := append([]string(nil), resources...)
	sort.Strings(sorted)
	return strings.ToLower(tenantID) + "|" + category + "|" + strings.Join(sorted, ",")
}

func routeKey(protocol, message string) string {
	return strings.ToLower(protocol) + "/" + strings.ToUpper(message)
}

func ruleCooldownKey(tenantID, ruleID string) string {
	return strings.ToLower(strings.TrimSpace(tenantID)) + "|rule|" + strings.TrimSpace(ruleID)
}

// Rules returns the compiled alert rules for inspection APIs.
func (c *AlertController) Rules() []AlertRuleDescriptor {
	if c == nil {
		return nil
	}
	descriptors := make([]AlertRuleDescriptor, 0, len(c.rules))
	for _, rule := range c.rules {
		if rule.id == "" {
			continue
		}
		desc := AlertRuleDescriptor{
			ID:              rule.id,
			Expression:      rule.expr,
			Severity:        string(rule.severity),
			SummaryTemplate: rule.summary,
			DetailTemplate:  rule.detail,
			Channels:        append([]string(nil), rule.channels...),
		}
		if rule.ttl > 0 {
			desc.TTLSeconds = int64(rule.ttl.Seconds())
		}
		descriptors = append(descriptors, desc)
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].ID < descriptors[j].ID
	})
	return descriptors
}

type policySettings struct {
	tenantID            string
	enabled             bool
	silenceWindow       time.Duration
	poolSampleSize      int
	poolThresholds      config.PoolThresholdConfig
	latencyThresholds   config.LatencyThresholdConfig
	errorThresholds     config.ErrorRateThresholdConfig
	rateLimitThresholds config.RateLimitAlertConfig
}
