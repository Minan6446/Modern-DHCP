package monitoring

import (
	"testing"
	"time"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
)

func TestCompileRuleNormalizesFields(t *testing.T) {
	cfg := config.AlertRuleConfig{
		ID:         "  burst-latency  ",
		Expression: "pool.utilization.max >= 80",
		Severity:   "major",
		Summary:    "Hot pool {{ pool.utilization.max }}%",
		Detail:     "Headroom {{ pool.headroom.min }}",
		Channels:   []string{" EmailOps ", "Webhook"},
		TTL:        2 * time.Minute,
	}
	rule, err := compileRule(cfg)
	if err != nil {
		t.Fatalf("compileRule returned error: %v", err)
	}
	if rule.id != "burst-latency" {
		t.Fatalf("expected trimmed id, got %q", rule.id)
	}
	if rule.severity != alerting.SeverityMajor {
		t.Fatalf("expected severity major, got %v", rule.severity)
	}
	if len(rule.channels) != 2 || rule.channels[0] != "emailops" || rule.channels[1] != "webhook" {
		t.Fatalf("expected normalized channels, got %#v", rule.channels)
	}
	if len(rule.clauses) != 1 {
		t.Fatalf("expected single clause, got %d", len(rule.clauses))
	}
	if rule.ttl != 2*time.Minute {
		t.Fatalf("expected ttl preserved, got %v", rule.ttl)
	}
}

func TestCompileRuleRequiresID(t *testing.T) {
	_, err := compileRule(config.AlertRuleConfig{Expression: "metric > 1"})
	if err == nil {
		t.Fatalf("expected error for missing id")
	}
}

func TestCompiledRuleMatches(t *testing.T) {
	rule := compiledRule{clauses: []ruleClause{
		{metric: "pool.utilization.max", op: ">=", value: 90},
		{metric: "request.errorRate.max", op: ">", value: 5},
	}}
	metrics := map[string]float64{
		"pool.utilization.max":  92,
		"request.errorrate.max": 6,
	}
	if !rule.matches(metrics) {
		t.Fatalf("expected rule to match metrics")
	}
	metrics["request.errorrate.max"] = 4
	if rule.matches(metrics) {
		t.Fatalf("expected rule to fail after lowering metric")
	}
}

func TestBuildRuleMetricsAggregates(t *testing.T) {
	pools := []PoolUsageSummary{
		{PoolID: "a", Utilization: 80, Capacity: 1000, Allocated: 800},
		{PoolID: "b", Utilization: 50, Capacity: 500, Allocated: 200},
	}
	requests := []RequestPhaseSnapshot{
		{AverageMs: 100, P95Ms: 300, Success: 900, Failure: 100},
		{AverageMs: 50, P95Ms: 120, Success: 500, Failure: 0},
	}
	security := SecuritySnapshot{RateLimit: RateLimitWindow{TotalHits: 12, UniqueMACs: 3, UniquePorts: 2}}
	metrics := buildRuleMetrics(pools, requests, security)
	if metrics["pool.count"] != 2 {
		t.Fatalf("expected pool count 2, got %.1f", metrics["pool.count"])
	}
	if metrics["pool.utilization.max"] != 80 {
		t.Fatalf("expected max utilization 80, got %.1f", metrics["pool.utilization.max"])
	}
	if metrics["pool.utilization.avg"] != 65 {
		t.Fatalf("expected avg utilization 65, got %.1f", metrics["pool.utilization.avg"])
	}
	if metrics["pool.headroom.min"] != 200 {
		t.Fatalf("expected min headroom 200, got %.1f", metrics["pool.headroom.min"])
	}
	if metrics["request.count"] != 2 {
		t.Fatalf("expected request count 2, got %.1f", metrics["request.count"])
	}
	if metrics["request.latency.p95.max"] != 300 {
		t.Fatalf("expected p95 max 300, got %.1f", metrics["request.latency.p95.max"])
	}
	if metrics["request.latency.avg"] != 75 {
		t.Fatalf("expected avg latency 75, got %.1f", metrics["request.latency.avg"])
	}
	if metrics["request.errorRate.max"] != 10 {
		t.Fatalf("expected error rate max 10, got %.1f", metrics["request.errorRate.max"])
	}
	if metrics["security.ratelimit.hits"] != 12 {
		t.Fatalf("expected rate limit hits 12, got %.1f", metrics["security.ratelimit.hits"])
	}
}

func TestRenderTemplateReplacesPlaceholders(t *testing.T) {
	template := "Pool hottest {{ pool.utilization.max }}% (hits {{ security.ratelimit.hits }})"
	metrics := map[string]float64{
		"pool.utilization.max":    92,
		"security.ratelimit.hits": 5,
	}
	rendered := renderTemplate(template, metrics)
	expected := "Pool hottest 92.00% (hits 5.00)"
	if rendered != expected {
		t.Fatalf("expected %q, got %q", expected, rendered)
	}
}
