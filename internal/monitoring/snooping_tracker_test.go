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

func TestSnoopingTrackerEvents(t *testing.T) {
	tracker := NewSnoopingTracker(5)
	base := time.Now().UTC()
	tracker.Record(snooping.Observation{TenantID: "tenant-z", Result: snooping.ObservationResultTrusted, PortID: "gi1/0/1", MAC: "aa:bb", OccurredAt: base.Add(-3 * time.Minute)})
	tracker.Record(snooping.Observation{TenantID: "tenant-z", Result: snooping.ObservationResultMiss, PortID: "gi1/0/2", MAC: "cc:dd", VLANID: 101, Reason: "no-entry", OccurredAt: base.Add(-40 * time.Second)})
	tracker.Record(snooping.Observation{TenantID: "tenant-z", Result: snooping.ObservationResultUntrusted, PortID: "gi1/0/3", MAC: "ee:ff", VLANID: 102, Reason: "rogue-device", OccurredAt: base.Add(-5 * time.Second)})

	events := tracker.Events("tenant-z", time.Minute, 2)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Result != snooping.ObservationResultMiss || events[1].Result != snooping.ObservationResultUntrusted {
		t.Fatalf("unexpected event results: %#v", events)
	}
	if events[0].VLANID != 101 || events[1].Reason != "rogue-device" {
		t.Fatalf("expected vlan and reason metadata retained, got %#v", events)
	}
}
