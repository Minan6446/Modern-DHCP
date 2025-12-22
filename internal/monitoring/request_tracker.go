package monitoring

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// RequestRecorder observes DHCP request phases.
type RequestRecorder interface {
	ObserveRequest(tenantID, protocol, message string, success bool, duration time.Duration)
}

// RequestTracker exposes snapshots for dashboards.
type RequestTracker interface {
	RequestRecorder
	Snapshot(tenantID string) []RequestPhaseSnapshot
}

// tracker implements RequestTracker with in-memory aggregates.
type tracker struct {
	mu      sync.RWMutex
	buckets map[string]*requestPhaseStats
}

// NewRequestTracker builds a thread-safe tracker instance.
func NewRequestTracker() RequestTracker {
	return &tracker{buckets: make(map[string]*requestPhaseStats)}
}

func (t *tracker) ObserveRequest(tenantID, protocol, message string, success bool, duration time.Duration) {
	key := trackerKey(tenantID, protocol, message)
	t.mu.Lock()
	bucket := t.buckets[key]
	if bucket == nil {
		bucket = &requestPhaseStats{
			protocol: protocol,
			message:  message,
			latency:  newLatencyHistogram(),
		}
		t.buckets[key] = bucket
	}
	if success {
		bucket.success++
	} else {
		bucket.failure++
	}
	bucket.latency.Observe(duration)
	t.mu.Unlock()
}

func (t *tracker) Snapshot(tenantID string) []RequestPhaseSnapshot {
	prefix := trackerKeyPrefix(tenantID)
	t.mu.RLock()
	results := make([]RequestPhaseSnapshot, 0)
	for key, stats := range t.buckets {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		results = append(results, stats.snapshot())
	}
	t.mu.RUnlock()
	sort.Slice(results, func(i, j int) bool {
		if results[i].Protocol == results[j].Protocol {
			return results[i].Message < results[j].Message
		}
		return results[i].Protocol < results[j].Protocol
	})
	return results
}

func trackerKey(tenantID, protocol, message string) string {
	return strings.ToLower(strings.TrimSpace(tenantID)) + "|" + strings.ToLower(protocol) + "|" + strings.ToUpper(message)
}

func trackerKeyPrefix(tenantID string) string {
	return strings.ToLower(strings.TrimSpace(tenantID)) + "|"
}

type requestPhaseStats struct {
	protocol string
	message  string
	success  uint64
	failure  uint64
	latency  *latencyHistogram
}

func (r *requestPhaseStats) snapshot() RequestPhaseSnapshot {
	avg := r.latency.AverageMillis()
	p95 := r.latency.PercentileMillis(0.95)
	return RequestPhaseSnapshot{
		Protocol:       r.protocol,
		Message:        r.message,
		Success:        r.success,
		Failure:        r.failure,
		AverageMs:      avg,
		P95Ms:          p95,
		LatencyBuckets: r.latency.Buckets(),
	}
}

type latencyHistogram struct {
	bounds []float64
	counts []uint64
	total  time.Duration
	seen   uint64
}

func newLatencyHistogram() *latencyHistogram {
	bounds := []float64{0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000}
	return &latencyHistogram{bounds: bounds, counts: make([]uint64, len(bounds)+1)}
}

func (h *latencyHistogram) Observe(d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	idx := len(h.counts) - 1
	for i, upper := range h.bounds {
		if ms <= upper {
			idx = i
			break
		}
	}
	h.counts[idx]++
	h.total += d
	h.seen++
}

func (h *latencyHistogram) AverageMillis() float64 {
	if h.seen == 0 {
		return 0
	}
	return float64(h.total) / float64(time.Millisecond) / float64(h.seen)
}

func (h *latencyHistogram) PercentileMillis(p float64) float64 {
	if h.seen == 0 || p <= 0 {
		return 0
	}
	if p >= 1 {
		p = 1
	}
	threshold := uint64(float64(h.seen) * p)
	if threshold == 0 {
		threshold = 1
	}
	var cumulative uint64
	for i, count := range h.counts {
		cumulative += count
		if cumulative >= threshold {
			if i >= len(h.bounds) {
				return h.bounds[len(h.bounds)-1]
			}
			return h.bounds[i]
		}
	}
	return h.bounds[len(h.bounds)-1]
}

func (h *latencyHistogram) Buckets() []LatencyBucket {
	buckets := make([]LatencyBucket, 0, len(h.counts))
	for i, count := range h.counts {
		upper := -1.0
		if i < len(h.bounds) {
			upper = h.bounds[i]
		}
		buckets = append(buckets, LatencyBucket{
			UpperBoundMs: upper,
			Count:        count,
		})
	}
	return buckets
}
