package monitoring

import (
	"testing"
	"time"
)

func TestRequestTrackerSnapshotAggregatesAndSorts(t *testing.T) {
	tracker := NewRequestTracker()

	tracker.ObserveRequest("tenantA", "dhcpv4", "DISCOVER", true, 5*time.Millisecond)
	tracker.ObserveRequest("tenantA", "dhcpv4", "DISCOVER", false, 15*time.Millisecond)
	tracker.ObserveRequest("tenantA", "dhcpv4", "REQUEST", true, 20*time.Millisecond)
	tracker.ObserveRequest("tenantA", "dhcpv6", "SOLICIT", true, 10*time.Millisecond)

	snapshots := tracker.Snapshot("tenantA")
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
	}

	var discover RequestPhaseSnapshot
	for _, snap := range snapshots {
		if snap.Message == "DISCOVER" {
			discover = snap
			break
		}
	}
	if discover.Success != 1 || discover.Failure != 1 {
		t.Fatalf("unexpected DISCOVER counts: %+v", discover)
	}
	if discover.AverageMs <= 0 {
		t.Fatalf("expected average latency > 0 got %.2f", discover.AverageMs)
	}
	if len(discover.LatencyBuckets) == 0 {
		t.Fatalf("expected latency buckets to be populated")
	}
}
