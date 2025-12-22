package monitoring

import (
	"testing"
	"time"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
)

func TestPickSeverity(t *testing.T) {
	thresholds := config.PoolThresholdConfig{Warning: 70, Major: 80, Critical: 90}
	tests := []struct {
		name      string
		util      float64
		want      alerting.Severity
		shouldHit bool
	}{
		{"below", 65, "", false},
		{"warning", 72, alerting.SeverityWarning, true},
		{"major", 84, alerting.SeverityMajor, true},
		{"critical", 95, alerting.SeverityCritical, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pickSeverity(tt.util, thresholds)
			if ok != tt.shouldHit {
				t.Fatalf("expected hit=%v got %v", tt.shouldHit, ok)
			}
			if got != tt.want {
				t.Fatalf("expected severity %v got %v", tt.want, got)
			}
		})
	}
}

func TestPickLatencySeverity(t *testing.T) {
	thresholds := config.LatencyThresholdConfig{Major: 200, Critical: 500}
	if sev, ok := pickLatencySeverity(150, thresholds); ok {
		t.Fatalf("expected no severity, got %v", sev)
	}
	sev, ok := pickLatencySeverity(250, thresholds)
	if !ok || sev != alerting.SeverityMajor {
		t.Fatalf("expected major severity, got %v ok=%v", sev, ok)
	}
	sev, ok = pickLatencySeverity(700, thresholds)
	if !ok || sev != alerting.SeverityCritical {
		t.Fatalf("expected critical severity, got %v ok=%v", sev, ok)
	}
}

func TestPickErrorSeverity(t *testing.T) {
	thresholds := config.ErrorRateThresholdConfig{Warning: 5, Major: 10, Critical: 25}
	if sev, ok := pickErrorSeverity(100, 0, thresholds); ok {
		t.Fatalf("expected no severity, got %v", sev)
	}
	sev, ok := pickErrorSeverity(94, 6, thresholds)
	if !ok || sev != alerting.SeverityWarning {
		t.Fatalf("expected warning, got %v ok=%v", sev, ok)
	}
	sev, ok = pickErrorSeverity(90, 10, thresholds)
	if !ok || sev != alerting.SeverityMajor {
		t.Fatalf("expected major, got %v ok=%v", sev, ok)
	}
	sev, ok = pickErrorSeverity(70, 30, thresholds)
	if !ok || sev != alerting.SeverityCritical {
		t.Fatalf("expected critical, got %v ok=%v", sev, ok)
	}
}

func TestPickRateLimitSeverity(t *testing.T) {
	thresholds := config.RateLimitAlertConfig{Warning: 10, Major: 25, Critical: 60}
	if sev, ok := pickRateLimitSeverity(5, thresholds); ok {
		t.Fatalf("expected no severity, got %v", sev)
	}
	sev, ok := pickRateLimitSeverity(12, thresholds)
	if !ok || sev != alerting.SeverityWarning {
		t.Fatalf("expected warning, got %v ok=%v", sev, ok)
	}
	sev, ok = pickRateLimitSeverity(30, thresholds)
	if !ok || sev != alerting.SeverityMajor {
		t.Fatalf("expected major, got %v", sev)
	}
	sev, ok = pickRateLimitSeverity(75, thresholds)
	if !ok || sev != alerting.SeverityCritical {
		t.Fatalf("expected critical, got %v", sev)
	}
}

func TestMergePolicyOverride(t *testing.T) {
	baseEnabled := true
	overrideEnabled := false
	base := config.AlertPolicyConfig{
		Enabled:        &baseEnabled,
		SilenceWindow:  5 * time.Minute,
		PoolSampleSize: 50,
		PoolThresholds: config.PoolThresholdConfig{Warning: 70, Major: 85, Critical: 95},
		Latency:        config.LatencyThresholdConfig{Major: 250, Critical: 500},
		ErrorRate:      config.ErrorRateThresholdConfig{Warning: 2, Major: 5, Critical: 10},
		RateLimit:      config.RateLimitAlertConfig{Window: time.Minute, Warning: 10, Major: 20, Critical: 40},
	}
	override := config.AlertPolicyConfig{
		Enabled:        &overrideEnabled,
		SilenceWindow:  2 * time.Minute,
		PoolSampleSize: 10,
		PoolThresholds: config.PoolThresholdConfig{Warning: 60},
		Latency:        config.LatencyThresholdConfig{Major: 150},
		ErrorRate:      config.ErrorRateThresholdConfig{Critical: 20},
		RateLimit:      config.RateLimitAlertConfig{Window: 30 * time.Second, Critical: 30},
	}
	merged := mergePolicy(base, override, "tenant-a")
	if merged.tenantID != "tenant-a" {
		t.Fatalf("expected tenant-a, got %s", merged.tenantID)
	}
	if merged.enabled {
		t.Fatalf("expected override to disable policy")
	}
	if merged.silenceWindow != 2*time.Minute {
		t.Fatalf("expected silence window override, got %v", merged.silenceWindow)
	}
	if merged.poolSampleSize != 10 {
		t.Fatalf("expected sample size override, got %d", merged.poolSampleSize)
	}
	if merged.poolThresholds.Warning != 60 {
		t.Fatalf("expected warning threshold override, got %.1f", merged.poolThresholds.Warning)
	}
	if merged.poolThresholds.Major != 85 {
		t.Fatalf("expected major threshold from base, got %.1f", merged.poolThresholds.Major)
	}
	if merged.latencyThresholds.Major != 150 {
		t.Fatalf("expected latency major override, got %.1f", merged.latencyThresholds.Major)
	}
	if merged.latencyThresholds.Critical != 500 {
		t.Fatalf("expected latency critical from base, got %.1f", merged.latencyThresholds.Critical)
	}
	if merged.errorThresholds.Critical != 20 {
		t.Fatalf("expected error critical override, got %.1f", merged.errorThresholds.Critical)
	}
	if merged.rateLimitThresholds.Window != 30*time.Second {
		t.Fatalf("expected rate limit window override, got %v", merged.rateLimitThresholds.Window)
	}
	if merged.rateLimitThresholds.Warning != 10 {
		t.Fatalf("expected warning to fall back to base, got %d", merged.rateLimitThresholds.Warning)
	}
	if merged.rateLimitThresholds.Critical != 30 {
		t.Fatalf("expected critical override, got %d", merged.rateLimitThresholds.Critical)
	}
}

func TestSilenceKeyDeterministic(t *testing.T) {
	key1 := silenceKey("Tenant-A", "POOL", []string{"pool-2", "pool-1"})
	key2 := silenceKey("tenant-a", "POOL", []string{"pool-1", "pool-2"})
	if key1 != key2 {
		t.Fatalf("expected deterministic silence key, got %s vs %s", key1, key2)
	}
}
