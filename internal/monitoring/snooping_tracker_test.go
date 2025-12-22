package monitoring

import (
	"testing"
	"time"

	"modern-dhcp/internal/security/snooping"
)

func TestSnoopingTrackerSnapshot(t *testing.T) {
	tracker := NewSnoopingTracker(4)
	now := time.Now().UTC()
	tracker.Record(snooping.Observation{TenantID: "Tenant-A", Result: snooping.ObservationResultTrusted, PortID: "gi1/0/1", MAC: "AA:BB", OccurredAt: now.Add(-4 * time.Minute)})
	tracker.Record(snooping.Observation{TenantID: "tenant-a", Result: snooping.ObservationResultUntrusted, PortID: "gi1/0/2", MAC: "cc:dd", Reason: "untrusted-port", OccurredAt: now.Add(-30 * time.Second)})

	snapshot := tracker.Snapshot("tenant-a", time.Minute)
	if snapshot.Total != 1 {
		t.Fatalf("expected only recent observation counted, got %d", snapshot.Total)
	}
	if snapshot.Untrusted != 1 || snapshot.Trusted != 0 {
		t.Fatalf("unexpected counters: %+v", snapshot)
	}
	if snapshot.LastPort != "gi1/0/2" || snapshot.LastMAC != "cc:dd" {
		t.Fatalf("expected last metadata updated")
	}
	if snapshot.LastResult != snooping.ObservationResultUntrusted {
		t.Fatalf("expected last result to track untrusted, got %s", snapshot.LastResult)
	}
}
