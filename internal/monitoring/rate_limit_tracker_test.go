package monitoring

import (
	"fmt"
	"testing"
	"time"

	"modern-dhcp/internal/security/ratelimit"
)

func TestRateLimitTrackerSnapshotWindow(t *testing.T) {
	tracker := NewRateLimitTracker(10)
	now := time.Now().UTC()
	tracker.Record(ratelimit.Hit{TenantID: "Tenant-A", MAC: "AA:BB", PortID: "gi1/0/1", IPAddress: "10.0.0.5", OccurredAt: now.Add(-10 * time.Second)})
	tracker.Record(ratelimit.Hit{TenantID: "tenant-a", MAC: "CC:DD", PortID: "gi1/0/2", IPAddress: "10.0.0.6", RetryAfter: 2 * time.Second, OccurredAt: now.Add(-2 * time.Second)})

	snapshot := tracker.Snapshot("tenant-a", 5*time.Second)
	if snapshot.TotalHits != 1 {
		t.Fatalf("expected 1 hit in window, got %d", snapshot.TotalHits)
	}
	if snapshot.UniqueMACs != 1 || snapshot.UniquePorts != 1 {
		t.Fatalf("expected single mac/port, got macs=%d ports=%d", snapshot.UniqueMACs, snapshot.UniquePorts)
	}
	if snapshot.LastMAC != "cc:dd" {
		t.Fatalf("expected normalized mac, got %s", snapshot.LastMAC)
	}
	if snapshot.LastRetryAfter != 2*time.Second {
		t.Fatalf("expected retryAfter to propagate, got %v", snapshot.LastRetryAfter)
	}
}

func TestRateLimitTrackerLimit(t *testing.T) {
	tracker := NewRateLimitTracker(3)
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		tracker.Record(ratelimit.Hit{
			TenantID:   "tenant-b",
			MAC:        fmt.Sprintf("00:00:00:00:00:%02x", i),
			PortID:     "p",
			OccurredAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	snapshot := tracker.Snapshot("tenant-b", 30*time.Second)
	if snapshot.TotalHits != 3 {
		t.Fatalf("expected tracker to retain only 3 hits, got %d", snapshot.TotalHits)
	}
	if snapshot.LastMAC == "" {
		t.Fatalf("expected last mac to be populated")
	}
	if snapshot.LastHit.Before(base.Add(2 * time.Second)) {
		t.Fatalf("expected last hit to represent most recent entry")
	}
}

func TestRateLimitTrackerEvents(t *testing.T) {
	tracker := NewRateLimitTracker(10)
	base := time.Now().UTC()
	for i := 0; i < 4; i++ {
		tracker.Record(ratelimit.Hit{
			TenantID:   "tenant-c",
			MAC:        fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i),
			PortID:     fmt.Sprintf("gi1/0/%d", i+1),
			IPAddress:  fmt.Sprintf("10.0.0.%d", i+1),
			RetryAfter: time.Duration(i) * time.Second,
			OccurredAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	events := tracker.Events("tenant-c", 3*time.Second, 2)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].MAC != "aa:bb:cc:dd:ee:02" || events[1].MAC != "aa:bb:cc:dd:ee:03" {
		t.Fatalf("unexpected event ordering: %#v", events)
	}
	if events[1].RetryAfter != 3*time.Second {
		t.Fatalf("expected retry after to be preserved")
	}
	if events[0].IP == "" || events[1].PortID == "" {
		t.Fatalf("expected ip and port to be populated")
	}
}
