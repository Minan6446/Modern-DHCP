package monitoring

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"

	"modern-dhcp/internal/lease"
)

// Analytics builds anomaly and capacity planning insights for the tenant.
func (a *Aggregator) Analytics(ctx context.Context, scope lease.ResourceScope, limit int) (AnalyticsSnapshot, error) {
	snapshot := AnalyticsSnapshot{GeneratedAt: time.Now().UTC()}
	if a == nil {
		return snapshot, errors.New("monitoring: aggregator unavailable")
	}
	usage, _, err := a.poolUsage(ctx, scope, limit)
	if err != nil {
		return snapshot, err
	}
	requests := a.requestSnapshots(scope)
	security := a.securitySnapshot(scope)
	snapshot.Capacity = buildCapacityInsights(usage)
	snapshot.Anomalies = detectAnomalies(usage, requests, security)
	return snapshot, nil
}

func buildCapacityInsights(usage []PoolUsageSummary) []CapacityInsight {
	insights := make([]CapacityInsight, 0, len(usage))
	for _, pool := range usage {
		headroom := pool.Capacity - pool.Allocated
		if headroom < 0 {
			headroom = 0
		}
		projection := forecastExhaustion(pool.Allocated, headroom)
		recommendation := recommendCapacityAction(pool.Utilization)
		insights = append(insights, CapacityInsight{
			PoolID:              pool.PoolID,
			Name:                pool.Name,
			Utilization:         pool.Utilization,
			Headroom:            headroom,
			ProjectedExhaustion: projection,
			Recommendation:      recommendation,
		})
	}
	sort.Slice(insights, func(i, j int) bool {
		return insights[i].Utilization > insights[j].Utilization
	})
	return insights
}

func detectAnomalies(pools []PoolUsageSummary, requests []RequestPhaseSnapshot, security SecuritySnapshot) []AnomalyInsight {
	anomalies := make([]AnomalyInsight, 0, 8)
	for _, pool := range pools {
		if pool.Utilization >= 85 {
			severity := "WARNING"
			if pool.Utilization >= 95 {
				severity = "CRITICAL"
			} else if pool.Utilization >= 90 {
				severity = "MAJOR"
			}
			anomalies = append(anomalies, AnomalyInsight{
				ID:         "capacity-" + pool.PoolID,
				Kind:       "capacity_hotspot",
				Metric:     "pool.utilization",
				Severity:   severity,
				Current:    pool.Utilization,
				Baseline:   80,
				Summary:    pool.Name + " utilization " + formatPercent(pool.Utilization),
				DetectedAt: time.Now().UTC(),
			})
		}
	}
	for _, snapshot := range requests {
		total := snapshot.Success + snapshot.Failure
		if total == 0 {
			continue
		}
		errorRate := (float64(snapshot.Failure) / float64(total)) * 100
		if errorRate >= 5 {
			severity := "WARNING"
			if errorRate >= 15 {
				severity = "MAJOR"
			}
			anomalies = append(anomalies, AnomalyInsight{
				ID:         "error-" + snapshot.Protocol + "-" + snapshot.Message,
				Kind:       "error_rate",
				Metric:     "request.errorRate",
				Severity:   severity,
				Current:    errorRate,
				Baseline:   2,
				Summary:    snapshot.Protocol + " " + snapshot.Message + " failure " + formatPercent(errorRate),
				DetectedAt: time.Now().UTC(),
			})
		}
		if snapshot.P95Ms >= math.Max(400, snapshot.AverageMs*3) {
			severity := "MAJOR"
			if snapshot.P95Ms >= 800 {
				severity = "CRITICAL"
			}
			anomalies = append(anomalies, AnomalyInsight{
				ID:         "latency-" + snapshot.Protocol + "-" + snapshot.Message,
				Kind:       "latency_spike",
				Metric:     "request.latency.p95",
				Severity:   severity,
				Current:    snapshot.P95Ms,
				Baseline:   snapshot.AverageMs,
				Summary:    snapshot.Protocol + " " + snapshot.Message + " p95 " + formatMillis(snapshot.P95Ms),
				DetectedAt: time.Now().UTC(),
			})
		}
	}
	if hits := security.RateLimit.TotalHits; hits >= 10 {
		severity := "WARNING"
		if hits >= 50 {
			severity = "MAJOR"
		}
		anomalies = append(anomalies, AnomalyInsight{
			ID:         "ratelimit",
			Kind:       "security_ratelimit",
			Metric:     "security.ratelimit.hits",
			Severity:   severity,
			Current:    float64(hits),
			Baseline:   5,
			Summary:    formatHits(hits) + " guard rate-limit hits",
			DetectedAt: security.RateLimit.LastHit,
		})
	}
	sort.SliceStable(anomalies, func(i, j int) bool {
		return anomalies[i].Severity > anomalies[j].Severity
	})
	return anomalies
}

func forecastExhaustion(allocated, headroom int64) time.Time {
	if headroom <= 0 {
		return time.Now().UTC()
	}
	burnRate := math.Max(1, float64(allocated)*0.02)
	days := float64(headroom) / burnRate
	if math.IsInf(days, 0) || math.IsNaN(days) {
		return time.Time{}
	}
	return time.Now().UTC().Add(time.Duration(days*24) * time.Hour)
}

func recommendCapacityAction(util float64) string {
	switch {
	case util >= 95:
		return "立即扩容/迁移租约"
	case util >= 90:
		return "规划扩容窗口"
	case util >= 80:
		return "监控增长趋势"
	default:
		return "容量充足"
	}
}

func formatPercent(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64) + "%"
}

func formatMillis(value float64) string {
	return strconv.FormatFloat(value, 'f', 0, 64) + "ms"
}

func formatHits(hits int) string {
	return strconv.Itoa(hits)
}
