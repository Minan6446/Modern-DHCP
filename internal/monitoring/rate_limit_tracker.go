package monitoring

import (
	"strings"
	"sync"
	"time"

	"modern-dhcp/internal/security/ratelimit"
)

// RateLimitTracker observes guard rejections and produces time-windowed snapshots.
type RateLimitTracker interface {
	ratelimit.Observer
	Snapshot(tenantID string, window time.Duration) RateLimitWindow
	Events(tenantID string, window time.Duration, limit int) []RateLimitEvent
}

// RateLimitWindow summarizes guard rate-limit activity.
type RateLimitWindow struct {
	TenantID       string        `json:"tenantId"`
	Window         time.Duration `json:"window"`
	TotalHits      int           `json:"totalHits"`
	UniqueMACs     int           `json:"uniqueMacs"`
	UniquePorts    int           `json:"uniquePorts"`
	LastMAC        string        `json:"lastMac"`
	LastPort       string        `json:"lastPort"`
	LastIP         string        `json:"lastIp"`
	LastRetryAfter time.Duration `json:"lastRetryAfter"`
	LastHit        time.Time     `json:"lastHit"`
}

type rateLimitTracker struct {
	mu        sync.Mutex
	perTenant map[string][]rateLimitEvent
	limit     int
}

type rateLimitEvent struct {
	occurredAt time.Time
	mac        string
	port       string
	ip         string
	retryAfter time.Duration
}

// NewRateLimitTracker returns an in-memory tracker with the provided per-tenant cap.
func NewRateLimitTracker(limit int) RateLimitTracker {
	if limit <= 0 {
		limit = 512
	}
	return &rateLimitTracker{perTenant: make(map[string][]rateLimitEvent), limit: limit}
}

// Record stores a guard rejection for later evaluation.
func (t *rateLimitTracker) Record(hit ratelimit.Hit) {
	if t == nil {
		return
	}
	canonical := normalizeTenant(hit.TenantID)
	tenantKey := strings.ToLower(canonical)
	evt := rateLimitEvent{
		occurredAt: hit.OccurredAt,
		mac:        strings.ToLower(strings.TrimSpace(hit.MAC)),
		port:       strings.TrimSpace(hit.PortID),
		ip:         strings.TrimSpace(hit.IPAddress),
		retryAfter: hit.RetryAfter,
	}
	if evt.occurredAt.IsZero() {
		evt.occurredAt = time.Now().UTC()
	}
	t.mu.Lock()
	bucket := append(t.perTenant[tenantKey], evt)
	if len(bucket) > t.limit {
		bucket = append([]rateLimitEvent(nil), bucket[len(bucket)-t.limit:]...)
	}
	t.perTenant[tenantKey] = bucket
	t.mu.Unlock()
}

// Snapshot returns aggregated statistics for the tenant within the given window.
func (t *rateLimitTracker) Snapshot(tenantID string, window time.Duration) RateLimitWindow {
	canonical := normalizeTenant(tenantID)
	snapshot := RateLimitWindow{TenantID: canonical, Window: window}
	if t == nil {
		return snapshot
	}
	copyBuf := t.recentEvents(canonical, window)
	if len(copyBuf) == 0 {
		return snapshot
	}
	macs := make(map[string]struct{})
	ports := make(map[string]struct{})
	snapshot.TotalHits = len(copyBuf)
	var last rateLimitEvent
	for idx, evt := range copyBuf {
		if evt.mac != "" {
			macs[evt.mac] = struct{}{}
		}
		if evt.port != "" {
			ports[evt.port] = struct{}{}
		}
		if idx == len(copyBuf)-1 {
			last = evt
		}
		if evt.retryAfter > snapshot.LastRetryAfter {
			snapshot.LastRetryAfter = evt.retryAfter
		}
	}
	snapshot.UniqueMACs = len(macs)
	snapshot.UniquePorts = len(ports)
	snapshot.LastMAC = last.mac
	snapshot.LastPort = last.port
	snapshot.LastIP = last.ip
	snapshot.LastHit = last.occurredAt
	if window > 0 {
		cutoff := time.Now().UTC().Add(-window)
		if snapshot.LastHit.Before(cutoff) {
			snapshot.LastHit = time.Now().UTC()
		}
	}
	return snapshot
}

func (t *rateLimitTracker) Events(tenantID string, window time.Duration, limit int) []RateLimitEvent {
	canonical := normalizeTenant(tenantID)
	if t == nil {
		return nil
	}
	events := t.recentEvents(canonical, window)
	if len(events) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(events) {
		limit = len(events)
	}
	start := len(events) - limit
	selected := events[start:]
	result := make([]RateLimitEvent, len(selected))
	for idx, evt := range selected {
		result[idx] = RateLimitEvent{
			TenantID:   canonical,
			OccurredAt: evt.occurredAt,
			MAC:        evt.mac,
			PortID:     evt.port,
			IP:         evt.ip,
			RetryAfter: evt.retryAfter,
		}
	}
	return result
}

func (t *rateLimitTracker) recentEvents(tenantID string, window time.Duration) []rateLimitEvent {
	if t == nil {
		return nil
	}
	key := strings.ToLower(tenantID)
	var cutoff time.Time
	if window > 0 {
		cutoff = time.Now().UTC().Add(-window)
	}
	t.mu.Lock()
	bucket := t.perTenant[key]
	if len(bucket) > 0 && !cutoff.IsZero() {
		prune := 0
		for prune < len(bucket) && bucket[prune].occurredAt.Before(cutoff) {
			prune++
		}
		if prune > 0 {
			bucket = append([]rateLimitEvent(nil), bucket[prune:]...)
			t.perTenant[key] = bucket
		}
	}
	copyBuf := make([]rateLimitEvent, len(bucket))
	copy(copyBuf, bucket)
	t.mu.Unlock()
	return copyBuf
}
