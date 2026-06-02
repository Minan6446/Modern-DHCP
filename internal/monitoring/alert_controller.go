package monitoring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/lease"
)

// AlertControllerOptions wires config + dependencies for the alert loop.
type AlertControllerOptions struct {
	Config      config.AlertingConfig
	Aggregator  *Aggregator
	RateTracker RateLimitTracker
	ConfigStore RuntimeAlertConfigStore
	Manager     *alerting.Manager
	Observer    AlertObserver
	Logger      *zap.Logger
}

// RuntimeAlertConfigStore loads alerting overrides from persisted alert configuration.
type RuntimeAlertConfigStore interface {
	GetThresholds(ctx context.Context, tenantID string) (alerting.Thresholds, error)
	GetNotify(ctx context.Context, tenantID string) (alerting.NotifyConfig, error)
	ListTemplates(ctx context.Context, tenantID string) ([]alerting.Template, error)
	ListReceivers(ctx context.Context, tenantID string) ([]alerting.Receiver, error)
}

const globalTenantID = "global"

// AlertController evaluates monitoring snapshots and emits alerts.
type AlertController struct {
	cfg          config.AlertingConfig
	aggregator   *Aggregator
	rateTracker  RateLimitTracker
	configStore  RuntimeAlertConfigStore
	manager      *alerting.Manager
	observer     AlertObserver
	logger       *zap.Logger
	baseRules    []compiledRule
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
	compiled := make([]compiledRule, 0, len(opts.Config.Rules))
	for _, ruleCfg := range opts.Config.Rules {
		rule, err := compileRule(ruleCfg)
		if err != nil {
			logger.Warn("alert rule skipped", zap.String("ruleId", ruleCfg.ID), zap.Error(err))
			continue
		}
		compiled = append(compiled, rule)
	}
	return &AlertController{
		cfg:          opts.Config,
		aggregator:   opts.Aggregator,
		rateTracker:  opts.RateTracker,
		configStore:  opts.ConfigStore,
		manager:      opts.Manager,
		observer:     opts.Observer,
		logger:       logger,
		baseRules:    compiled,
		policies:     policies,
		silence:      make(map[string]time.Time),
		rules:        compiled,
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
		effective := c.applyRuntimeConfig(ctx, policy)
		c.evaluatePools(ctx, effective)
		c.evaluateRequests(ctx, effective)
		c.evaluateRateLimits(ctx, effective)
		c.evaluateRules(ctx, effective)
		c.evaluateOperationalThresholds(ctx, effective)
	}
}

func (c *AlertController) applyRuntimeConfig(ctx context.Context, base policySettings) policySettings {
	if c == nil || c.configStore == nil {
		return base
	}
	tenantID := strings.TrimSpace(base.tenantID)
	if tenantID == "" {
		tenantID = globalTenantID
	}
	if thresholds, err := c.configStore.GetThresholds(ctx, tenantID); err == nil {
		base.runtimeThresholds = runtimeThresholdSettings{
			ResourcePoolUsage:           thresholds.ResourcePoolUsage,
			ResourceLeaseUsage:          thresholds.ResourceLeaseUsage,
			ResourceRenewFail:           thresholds.ResourceRenewFail,
			ResourceLeaseTimeDrift:      thresholds.ResourceLeaseTimeDrift,
			ResourceFailedRequestRatio:  thresholds.ResourceFailedRequestRatio,
			ResourceSubnetImbalance:     thresholds.ResourceSubnetImbalance,
			ResourceLogErrorThreshold:   thresholds.ResourceLogErrorThreshold,
			ServerResponseTimeout:       thresholds.ServerResponseTimeout,
			ServerResponseTimeMs:        thresholds.ServerResponseTimeMs,
			ServerCPUUsage:              thresholds.ServerCPUUsage,
			ServerMemoryUsage:           thresholds.ServerMemoryUsage,
			ServerProcessCheck:          thresholds.ServerProcessCheck,
			NetworkConflictSensitivity:  strings.ToLower(strings.TrimSpace(thresholds.NetworkConflictSensitivity)),
			NetworkAbnormalQPS:          thresholds.NetworkAbnormalQPS,
			NetworkDuplicateIPDetection: thresholds.NetworkDuplicateIPDetection,
			NetworkUnauthorizedDHCP:     thresholds.NetworkUnauthorizedDHCP,
			enabled:                     true,
		}
		base.poolThresholds = mergeRuntimePoolThresholds(base.poolThresholds, base.runtimeThresholds)
		base.latencyThresholds = mergeRuntimeLatencyThresholds(base.latencyThresholds, base.runtimeThresholds)
		base.errorThresholds = mergeRuntimeErrorThresholds(base.errorThresholds, base.runtimeThresholds)
		base.rateLimitThresholds = mergeRuntimeRateLimitThresholds(base.rateLimitThresholds, base.runtimeThresholds)
	} else if !errors.Is(err, sql.ErrNoRows) {
		c.logger.Warn("load alert runtime thresholds failed", zap.String("tenant", tenantID), zap.Error(err))
	}

	if notifyCfg, err := c.configStore.GetNotify(ctx, tenantID); err == nil {
		base.notifyChannelsBySeverity = buildNotifyChannelsBySeverity(notifyCfg)
		base.runtimeWebhookURL = strings.TrimSpace(notifyCfg.WebhookURL)
	} else if !errors.Is(err, sql.ErrNoRows) {
		c.logger.Warn("load alert runtime notify policy failed", zap.String("tenant", tenantID), zap.Error(err))
	}

	if templates, err := c.configStore.ListTemplates(ctx, tenantID); err == nil {
		base.templates = templates
	} else if !errors.Is(err, sql.ErrNoRows) {
		c.logger.Warn("load alert runtime templates failed", zap.String("tenant", tenantID), zap.Error(err))
	}

	if receivers, err := c.configStore.ListReceivers(ctx, tenantID); err == nil {
		base.receivers = receivers
	} else if !errors.Is(err, sql.ErrNoRows) {
		c.logger.Warn("load alert runtime receivers failed", zap.String("tenant", tenantID), zap.Error(err))
	}

	return base
}

func (c *AlertController) evaluatePools(ctx context.Context, policy policySettings) {
	if policy.poolThresholds == (config.PoolThresholdConfig{}) {
		return
	}
	limit := policy.poolSampleSize
	if limit <= 0 {
		limit = 50
	}
	scope := policy.resourceScope()
	usage, err := c.aggregator.Pools(ctx, scope, limit)
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
	scope := policy.resourceScope()
	snapshots := c.aggregator.Requests(scope)
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
	c.mu.Lock()
	rules := append([]compiledRule(nil), c.rules...)
	c.mu.Unlock()
	limit := policy.poolSampleSize
	if limit <= 0 {
		limit = 50
	}
	scope := policy.resourceScope()
	usage, err := c.aggregator.Pools(ctx, scope, limit)
	if err != nil {
		c.logger.Warn("rule evaluation pool snapshot failed", zap.String("tenant", policy.tenantID), zap.Error(err))
		return
	}
	requests := c.aggregator.Requests(scope)
	security := c.aggregator.Security(scope)
	metrics := buildRuleMetrics(usage, requests, security)
	now := time.Now().UTC()
	for _, rule := range rules {
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
	if len(event.Channels) == 0 {
		event.Channels = policy.channelsForSeverity(event.Severity)
	}
	event = decorateEventWithRuntimeConfig(policy, event)
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

func (c *AlertController) evaluateOperationalThresholds(ctx context.Context, policy policySettings) {
	if c == nil || c.aggregator == nil || !policy.runtimeThresholds.enabled {
		return
	}
	thresholds := policy.runtimeThresholds
	scope := policy.resourceScope()

	if thresholds.ServerCPUUsage > 0 || thresholds.ServerMemoryUsage > 0 {
		health := c.aggregator.Health(ctx)
		if thresholds.ServerCPUUsage > 0 && health.CPUPercent >= float64(thresholds.ServerCPUUsage) {
			c.emit(ctx, policy, alerting.Event{
				Severity:  alerting.SeverityMajor,
				TenantID:  policy.tenantID,
				Category:  "SERVER_CPU_USAGE",
				Summary:   fmt.Sprintf("CPU usage %.1f%% exceeds threshold %d%%", health.CPUPercent, thresholds.ServerCPUUsage),
				Details:   fmt.Sprintf("CPU %.1f%%, configured threshold %d%%", health.CPUPercent, thresholds.ServerCPUUsage),
				Resources: []string{"system/cpu"},
			})
		}
		if thresholds.ServerMemoryUsage > 0 && health.MemoryPercent >= float64(thresholds.ServerMemoryUsage) {
			c.emit(ctx, policy, alerting.Event{
				Severity:  alerting.SeverityMajor,
				TenantID:  policy.tenantID,
				Category:  "SERVER_MEMORY_USAGE",
				Summary:   fmt.Sprintf("Memory usage %.1f%% exceeds threshold %d%%", health.MemoryPercent, thresholds.ServerMemoryUsage),
				Details:   fmt.Sprintf("Memory %.1f%%, configured threshold %d%%", health.MemoryPercent, thresholds.ServerMemoryUsage),
				Resources: []string{"system/memory"},
			})
		}
	}

	requests := c.aggregator.Requests(scope)
	if len(requests) > 0 {
		var totalSuccess, totalFailure uint64
		var maxP95Value float64
		var renewSuccess, renewFailure uint64
		for _, snap := range requests {
			totalSuccess += snap.Success
			totalFailure += snap.Failure
			if snap.P95Ms > maxP95Value {
				maxP95Value = snap.P95Ms
			}
			if strings.Contains(strings.ToLower(snap.Message), "renew") {
				renewSuccess += snap.Success
				renewFailure += snap.Failure
			}
		}
		if thresholds.ServerResponseTimeMs > 0 && maxP95Value >= float64(thresholds.ServerResponseTimeMs) {
			c.emit(ctx, policy, alerting.Event{
				Severity:  alerting.SeverityMajor,
				TenantID:  policy.tenantID,
				Category:  "REQUEST_RESPONSE_TIME",
				Summary:   fmt.Sprintf("Request p95 %.1fms exceeds threshold %dms", maxP95Value, thresholds.ServerResponseTimeMs),
				Details:   fmt.Sprintf("Maximum p95 latency %.1fms, configured threshold %dms", maxP95Value, thresholds.ServerResponseTimeMs),
				Resources: []string{"request/latency"},
			})
		}
		if thresholds.ServerResponseTimeout > 0 && maxP95Value >= float64(thresholds.ServerResponseTimeout*1000) {
			c.emit(ctx, policy, alerting.Event{
				Severity:  alerting.SeverityCritical,
				TenantID:  policy.tenantID,
				Category:  "REQUEST_TIMEOUT",
				Summary:   fmt.Sprintf("Request latency %.1fms exceeds timeout threshold %ds", maxP95Value, thresholds.ServerResponseTimeout),
				Details:   fmt.Sprintf("Maximum p95 latency %.1fms, timeout threshold %ds", maxP95Value, thresholds.ServerResponseTimeout),
				Resources: []string{"request/timeout"},
			})
		}
		total := totalSuccess + totalFailure
		if thresholds.ResourceFailedRequestRatio > 0 && total > 0 {
			rate := (float64(totalFailure) / float64(total)) * 100
			if rate >= float64(thresholds.ResourceFailedRequestRatio) {
				c.emit(ctx, policy, alerting.Event{
					Severity:  alerting.SeverityMajor,
					TenantID:  policy.tenantID,
					Category:  "REQUEST_FAILED_RATIO",
					Summary:   fmt.Sprintf("Request failure ratio %.1f%% exceeds threshold %d%%", rate, thresholds.ResourceFailedRequestRatio),
					Details:   fmt.Sprintf("Failures %d / total %d (%.1f%%)", totalFailure, total, rate),
					Resources: []string{"request/failure-ratio"},
				})
			}
		}
		if thresholds.ResourceRenewFail > 0 && (renewSuccess+renewFailure) > 0 {
			renewRate := (float64(renewFailure) / float64(renewSuccess+renewFailure)) * 100
			if renewRate >= float64(thresholds.ResourceRenewFail) {
				c.emit(ctx, policy, alerting.Event{
					Severity:  alerting.SeverityMajor,
					TenantID:  policy.tenantID,
					Category:  "RENEW_FAILURE_RATE",
					Summary:   fmt.Sprintf("Renew failure ratio %.1f%% exceeds threshold %d%%", renewRate, thresholds.ResourceRenewFail),
					Details:   fmt.Sprintf("Renew failures %d / total %d (%.1f%%)", renewFailure, renewSuccess+renewFailure, renewRate),
					Resources: []string{"request/renew"},
				})
			}
		}
		if thresholds.ResourceLogErrorThreshold > 0 && int(totalFailure) >= thresholds.ResourceLogErrorThreshold {
			c.emit(ctx, policy, alerting.Event{
				Severity:  alerting.SeverityMajor,
				TenantID:  policy.tenantID,
				Category:  "REQUEST_FAILURE_COUNT",
				Summary:   fmt.Sprintf("Request failure count %d exceeds threshold %d", totalFailure, thresholds.ResourceLogErrorThreshold),
				Details:   fmt.Sprintf("Failure events in current window: %d", totalFailure),
				Resources: []string{"request/failure-count"},
			})
		}
	}

	if pools, err := c.aggregator.Pools(ctx, scope, 50); err == nil {
		if thresholds.ResourceSubnetImbalance > 0 && len(pools) > 1 {
			maxUtil := pools[0].Utilization
			minUtil := pools[0].Utilization
			for _, item := range pools[1:] {
				if item.Utilization > maxUtil {
					maxUtil = item.Utilization
				}
				if item.Utilization < minUtil {
					minUtil = item.Utilization
				}
			}
			imbalance := maxUtil - minUtil
			if imbalance >= float64(thresholds.ResourceSubnetImbalance) {
				c.emit(ctx, policy, alerting.Event{
					Severity:  alerting.SeverityWarning,
					TenantID:  policy.tenantID,
					Category:  "POOL_SUBNET_IMBALANCE",
					Summary:   fmt.Sprintf("Subnet utilization imbalance %.1f%% exceeds threshold %d%%", imbalance, thresholds.ResourceSubnetImbalance),
					Details:   fmt.Sprintf("Max pool utilization %.1f%%, min %.1f%%", maxUtil, minUtil),
					Resources: []string{"pool/imbalance"},
				})
			}
		}
	}

	security := c.aggregator.Security(scope)
	if thresholds.NetworkAbnormalQPS > 0 && security.RateLimit.TotalHits >= thresholds.NetworkAbnormalQPS {
		sev := severityBySensitivity(thresholds.NetworkConflictSensitivity)
		c.emit(ctx, policy, alerting.Event{
			Severity:  sev,
			TenantID:  policy.tenantID,
			Category:  "NETWORK_ABNORMAL_QPS",
			Summary:   fmt.Sprintf("Abnormal request count %d exceeds threshold %d", security.RateLimit.TotalHits, thresholds.NetworkAbnormalQPS),
			Details:   fmt.Sprintf("Rate-limit hits in window %s: %d", security.RateLimit.Window, security.RateLimit.TotalHits),
			Resources: []string{"network/abnormal"},
		})
	}
	if thresholds.NetworkDuplicateIPDetection && security.Snooping.Misses > 0 {
		sev := severityBySensitivity(thresholds.NetworkConflictSensitivity)
		c.emit(ctx, policy, alerting.Event{
			Severity:  sev,
			TenantID:  policy.tenantID,
			Category:  "NETWORK_DUPLICATE_IP_DETECTED",
			Summary:   fmt.Sprintf("Potential duplicate IP events detected: %d", security.Snooping.Misses),
			Details:   fmt.Sprintf("Snooping misses in window %s: %d", security.Snooping.Window, security.Snooping.Misses),
			Resources: []string{"network/duplicate-ip"},
		})
	}
	if thresholds.NetworkUnauthorizedDHCP && security.Snooping.Untrusted > 0 {
		c.emit(ctx, policy, alerting.Event{
			Severity:  alerting.SeverityCritical,
			TenantID:  policy.tenantID,
			Category:  "NETWORK_UNAUTHORIZED_DHCP",
			Summary:   fmt.Sprintf("Unauthorized DHCP events detected: %d", security.Snooping.Untrusted),
			Details:   fmt.Sprintf("Untrusted snooping events in window %s: %d", security.Snooping.Window, security.Snooping.Untrusted),
			Resources: []string{"network/unauthorized-dhcp"},
		})
	}
}

func resolvePolicies(cfg config.AlertingConfig) []policySettings {
	settings := []policySettings{mergePolicy(cfg.DefaultPolicy, config.AlertPolicyConfig{}, globalTenantID)}
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

// Tenants returns the tenant IDs with configured alert policies.
func (c *AlertController) Tenants() []string {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.policies))
	for _, policy := range c.policies {
		ids = append(ids, policy.tenantID)
	}
	return ids
}

// SetRules replaces dynamic rules while preserving static config rules.
func (c *AlertController) SetRules(configs []config.AlertRuleConfig) {
	if c == nil {
		return
	}
	compiled := make([]compiledRule, 0, len(c.baseRules)+len(configs))
	compiled = append(compiled, c.baseRules...)
	for _, cfg := range configs {
		rule, err := compileRule(cfg)
		if err != nil {
			c.logger.Warn("alert rule skipped", zap.String("ruleId", cfg.ID), zap.Error(err))
			continue
		}
		compiled = append(compiled, rule)
	}
	c.mu.Lock()
	c.rules = compiled
	c.mu.Unlock()
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
			Name:            rule.summary,
			Expression:      rule.expr,
			Severity:        string(rule.severity),
			SummaryTemplate: rule.summary,
			DetailTemplate:  rule.detail,
			Channels:        append([]string(nil), rule.channels...),
		}
		if rule.ttl > 0 {
			desc.TTLSeconds = int64(rule.ttl.Seconds())
			desc.DurationSeconds = int(rule.ttl.Seconds())
		}
		if len(rule.clauses) > 0 {
			first := rule.clauses[0]
			desc.Operator = first.op
			desc.Threshold = first.value
		}
		descriptors = append(descriptors, desc)
	}
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].ID < descriptors[j].ID
	})
	return descriptors
}

type policySettings struct {
	tenantID                 string
	enabled                  bool
	silenceWindow            time.Duration
	poolSampleSize           int
	poolThresholds           config.PoolThresholdConfig
	latencyThresholds        config.LatencyThresholdConfig
	errorThresholds          config.ErrorRateThresholdConfig
	rateLimitThresholds      config.RateLimitAlertConfig
	runtimeThresholds        runtimeThresholdSettings
	notifyChannelsBySeverity map[alerting.Severity][]string
	runtimeWebhookURL        string
	templates                []alerting.Template
	receivers                []alerting.Receiver
}

func (p policySettings) resourceScope() lease.ResourceScope {
	return lease.NewResourceScope("", p.tenantID)
}

func (p policySettings) channelsForSeverity(sev alerting.Severity) []string {
	if len(p.notifyChannelsBySeverity) == 0 {
		return nil
	}
	channels := p.notifyChannelsBySeverity[sev]
	if len(channels) == 0 {
		return nil
	}
	dup := make([]string, 0, len(channels))
	for _, ch := range channels {
		trimmed := strings.ToLower(strings.TrimSpace(ch))
		if trimmed != "" {
			dup = append(dup, trimmed)
		}
	}
	return dup
}

type runtimeThresholdSettings struct {
	ResourcePoolUsage           int
	ResourceLeaseUsage          int
	ResourceRenewFail           int
	ResourceLeaseTimeDrift      int
	ResourceFailedRequestRatio  int
	ResourceSubnetImbalance     int
	ResourceLogErrorThreshold   int
	ServerResponseTimeout       int
	ServerResponseTimeMs        int
	ServerCPUUsage              int
	ServerMemoryUsage           int
	ServerProcessCheck          bool
	NetworkConflictSensitivity  string
	NetworkAbnormalQPS          int
	NetworkDuplicateIPDetection bool
	NetworkUnauthorizedDHCP     bool
	enabled                     bool
}

func mergeRuntimePoolThresholds(base config.PoolThresholdConfig, runtime runtimeThresholdSettings) config.PoolThresholdConfig {
	result := base
	if runtime.ResourcePoolUsage > 0 {
		result.Warning = float64(runtime.ResourcePoolUsage)
	}
	if runtime.ResourceLeaseUsage > 0 {
		result.Major = float64(runtime.ResourceLeaseUsage)
		if result.Critical <= 0 || result.Critical < result.Major {
			result.Critical = result.Major
		}
	}
	if result.Critical <= 0 && result.Major > 0 {
		result.Critical = result.Major
	}
	if result.Major <= 0 && result.Warning > 0 {
		result.Major = result.Warning
	}
	return result
}

func mergeRuntimeLatencyThresholds(base config.LatencyThresholdConfig, runtime runtimeThresholdSettings) config.LatencyThresholdConfig {
	result := base
	if runtime.ServerResponseTimeMs > 0 {
		result.Major = float64(runtime.ServerResponseTimeMs)
	}
	if runtime.ServerResponseTimeout > 0 {
		critical := float64(runtime.ServerResponseTimeout * 1000)
		if critical > result.Critical {
			result.Critical = critical
		}
	}
	if result.Critical <= 0 && result.Major > 0 {
		result.Critical = result.Major
	}
	return result
}

func mergeRuntimeErrorThresholds(base config.ErrorRateThresholdConfig, runtime runtimeThresholdSettings) config.ErrorRateThresholdConfig {
	result := base
	if runtime.ResourceFailedRequestRatio > 0 {
		result.Warning = float64(runtime.ResourceFailedRequestRatio)
	}
	if runtime.ResourceRenewFail > 0 {
		result.Major = float64(runtime.ResourceRenewFail)
	}
	if result.Major > 0 {
		result.Critical = result.Major
	} else if result.Warning > 0 {
		result.Critical = result.Warning
	}
	return result
}

func mergeRuntimeRateLimitThresholds(base config.RateLimitAlertConfig, runtime runtimeThresholdSettings) config.RateLimitAlertConfig {
	result := base
	if runtime.NetworkAbnormalQPS > 0 {
		result.Warning = runtime.NetworkAbnormalQPS
		result.Major = runtime.NetworkAbnormalQPS
		result.Critical = runtime.NetworkAbnormalQPS
	}
	return result
}

func buildNotifyChannelsBySeverity(cfg alerting.NotifyConfig) map[alerting.Severity][]string {
	enabled := map[string]bool{
		"email":   cfg.Channels.Email,
		"sms":     cfg.Channels.SMS,
		"webhook": cfg.Channels.Webhook,
	}
	filter := func(raw []string) []string {
		channels := make([]string, 0, len(raw))
		for _, ch := range raw {
			name := strings.ToLower(strings.TrimSpace(ch))
			if name == "" {
				continue
			}
			if ok, exists := enabled[name]; exists && !ok {
				continue
			}
			channels = append(channels, name)
		}
		return channels
	}
	return map[alerting.Severity][]string{
		alerting.SeverityCritical: filter(cfg.Policies.Emergency),
		alerting.SeverityMajor:    filter(cfg.Policies.Critical),
		alerting.SeverityWarning:  filter(cfg.Policies.Info),
		alerting.SeverityInfo:     filter(cfg.Policies.Info),
	}
}

func severityBySensitivity(s string) alerting.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high":
		return alerting.SeverityCritical
	case "medium":
		return alerting.SeverityMajor
	default:
		return alerting.SeverityWarning
	}
}

func decorateEventWithRuntimeConfig(policy policySettings, event alerting.Event) alerting.Event {
	if event.Labels == nil {
		event.Labels = map[string]string{}
	}
	if len(event.Channels) == 0 {
		event.Channels = policy.channelsForSeverity(event.Severity)
	}
	if len(policy.receivers) > 0 {
		emails, phones := resolveReceiverTargets(policy.receivers, event.Severity)
		if len(emails) > 0 {
			event.Labels["receiver_emails"] = strings.Join(emails, ",")
		}
		if len(phones) > 0 {
			event.Labels["receiver_phones"] = strings.Join(phones, ",")
		}
	}
	if policy.runtimeWebhookURL != "" {
		event.Labels["webhook_url"] = policy.runtimeWebhookURL
	}
	tpl := pickRuntimeTemplate(policy.templates, event.Channels)
	if tpl != nil {
		vars := buildTemplateVars(event)
		if subject := strings.TrimSpace(renderRuntimeTemplate(tpl.Subject, vars)); subject != "" {
			event.Summary = subject
		}
		if body := strings.TrimSpace(renderRuntimeTemplate(tpl.Body, vars)); body != "" {
			event.Details = body
		}
	}
	return event
}

func resolveReceiverTargets(receivers []alerting.Receiver, sev alerting.Severity) ([]string, []string) {
	level := severityToReceiverLevel(sev)
	emails := make([]string, 0)
	phones := make([]string, 0)
	emailSeen := make(map[string]struct{})
	phoneSeen := make(map[string]struct{})
	for _, r := range receivers {
		if !receiverMatchesLevel(r.Levels, level) {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if email != "" {
			if _, ok := emailSeen[email]; !ok {
				emailSeen[email] = struct{}{}
				emails = append(emails, email)
			}
		}
		phone := strings.TrimSpace(r.Phone)
		if phone != "" {
			if _, ok := phoneSeen[phone]; !ok {
				phoneSeen[phone] = struct{}{}
				phones = append(phones, phone)
			}
		}
	}
	return emails, phones
}

func severityToReceiverLevel(sev alerting.Severity) string {
	switch sev {
	case alerting.SeverityCritical:
		return "emergency"
	case alerting.SeverityMajor:
		return "critical"
	default:
		return "info"
	}
}

func receiverMatchesLevel(levels []string, target string) bool {
	if len(levels) == 0 {
		return true
	}
	for _, item := range levels {
		norm := strings.ToLower(strings.TrimSpace(item))
		if norm == "" {
			continue
		}
		if norm == "all" || norm == target {
			return true
		}
	}
	return false
}

func pickRuntimeTemplate(templates []alerting.Template, channels []string) *alerting.Template {
	if len(templates) == 0 || len(channels) == 0 {
		return nil
	}
	channelSet := make(map[string]struct{}, len(channels))
	for _, ch := range channels {
		norm := strings.ToLower(strings.TrimSpace(ch))
		if norm != "" {
			channelSet[norm] = struct{}{}
		}
	}
	var first *alerting.Template
	for i := range templates {
		tpl := &templates[i]
		channel := strings.ToLower(strings.TrimSpace(tpl.Channel))
		if channel == "" {
			continue
		}
		if _, ok := channelSet[channel]; !ok {
			continue
		}
		if first == nil {
			first = tpl
		}
		if strings.EqualFold(strings.TrimSpace(tpl.Lang), "zh-CN") {
			return tpl
		}
	}
	return first
}

func buildTemplateVars(event alerting.Event) map[string]string {
	vars := map[string]string{
		"severity":   string(event.Severity),
		"category":   event.Category,
		"summary":    event.Summary,
		"details":    event.Details,
		"tenantId":   event.TenantID,
		"occurredAt": event.OccurredAt.Format(time.RFC3339),
		"ruleName":   event.Category,
	}
	for k, v := range event.Labels {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		vars[key] = v
	}
	return vars
}

func renderRuntimeTemplate(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}
