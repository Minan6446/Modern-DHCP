package monitoring

import (
	"strings"
	"sync"
	"time"

	"modern-dhcp/internal/security/snooping"
)

// SnoopingTracker aggregates snooping observations per tenant.
type SnoopingTracker interface {
	snooping.Observer
	Snapshot(tenantID string, window time.Duration) SnoopingWindow
}

// SnoopingWindow summarizes snooping lookup outcomes for dashboards.
type SnoopingWindow struct {
	TenantID   string                     `json:"tenantId"`
	Window     time.Duration              `json:"window"`
	Total      int                        `json:"total"`
	Trusted    int                        `json:"trusted"`
	Misses     int                        `json:"misses"`
	Untrusted  int                        `json:"untrusted"`
	Errors     int                        `json:"errors"`
	LastResult snooping.ObservationResult `json:"lastResult"`
	LastPort   string                     `json:"lastPort"`
	LastMAC    string                     `json:"lastMac"`
	LastVLAN   int                        `json:"lastVlan"`
	LastReason string                     `json:"lastReason,omitempty"`
	LastEvent  time.Time                  `json:"lastEvent"`
}

type snoopingTracker struct {
	mu        sync.Mutex
	perTenant map[string][]snoopEvent
	limit     int
}

type snoopEvent struct {
	occurredAt time.Time
	result     snooping.ObservationResult
	reason     string
	port       string
	mac        string
	vlan       int
}

// NewSnoopingTracker returns an in-memory tracker with bounded history.
func NewSnoopingTracker(limit int) SnoopingTracker {
	if limit <= 0 {
		limit = 512
	}
	return &snoopingTracker{perTenant: make(map[string][]snoopEvent), limit: limit}
}

// Record implements snooping.Observer.
func (t *snoopingTracker) Record(obs snooping.Observation) {
	if t == nil {
		return
	}
	canonical := normalizeTenant(obs.TenantID)
	tenantKey := strings.ToLower(canonical)
	evt := snoopEvent{
		occurredAt: obs.OccurredAt,
		result:     obs.Result,
		reason:     strings.TrimSpace(obs.Reason),
		port:       strings.TrimSpace(obs.PortID),
		mac:        strings.ToLower(strings.TrimSpace(obs.MAC)),
		vlan:       obs.VLANID,
	}
	if evt.occurredAt.IsZero() {
		evt.occurredAt = time.Now().UTC()
	}
	t.mu.Lock()
	bucket := append(t.perTenant[tenantKey], evt)
	if len(bucket) > t.limit {
		bucket = append([]snoopEvent(nil), bucket[len(bucket)-t.limit:]...)
	}
	t.perTenant[tenantKey] = bucket
	t.mu.Unlock()
}

// Snapshot returns counts for the provided window.
func (t *snoopingTracker) Snapshot(tenantID string, window time.Duration) SnoopingWindow {
	canonical := normalizeTenant(tenantID)
	snap := SnoopingWindow{TenantID: canonical, Window: window}
	if t == nil || window <= 0 {
		return snap
	}
	cutoff := time.Now().UTC().Add(-window)
	t.mu.Lock()
	key := strings.ToLower(canonical)
	bucket := t.perTenant[key]
	if len(bucket) > 0 {
		prune := 0
		for prune < len(bucket) && bucket[prune].occurredAt.Before(cutoff) {
			prune++
		}
		if prune > 0 {
			bucket = append([]snoopEvent(nil), bucket[prune:]...)
			t.perTenant[key] = bucket
		}
	}
	copyBuf := make([]snoopEvent, len(bucket))
	copy(copyBuf, bucket)
	t.mu.Unlock()

	if len(copyBuf) == 0 {
		return snap
	}
	snap.Total = len(copyBuf)
	last := copyBuf[len(copyBuf)-1]
	for _, evt := range copyBuf {
		switch evt.result {
		case snooping.ObservationResultTrusted:
			snap.Trusted++
		case snooping.ObservationResultMiss:
			snap.Misses++
		case snooping.ObservationResultUntrusted:
			snap.Untrusted++
		default:
			snap.Errors++
		}
	}
	snap.LastResult = last.result
	snap.LastPort = last.port
	snap.LastMAC = last.mac
	snap.LastVLAN = last.vlan
	snap.LastReason = last.reason
	snap.LastEvent = last.occurredAt
	if snap.LastEvent.Before(cutoff) {
		snap.LastEvent = time.Now().UTC()
	}
	return snap
}
