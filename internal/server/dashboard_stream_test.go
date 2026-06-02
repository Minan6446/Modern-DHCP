package server

import (
	"testing"
	"time"

	"modern-dhcp/internal/dashboard"
)

func TestFilterNewStreamEntries(t *testing.T) {
	base := time.Now().UTC()
	entries := []dashboard.StreamEntry{
		{ID: "c", Type: dashboard.StreamTypeAlert, OccurredAt: base},
		{ID: "b", Type: dashboard.StreamTypeOperation, OccurredAt: base.Add(-1 * time.Second)},
		{ID: "a", Type: dashboard.StreamTypeAlert, OccurredAt: base.Add(-2 * time.Second)},
	}
	seen := newStreamSeenSet(10)
	fresh, latest := filterNewStreamEntries(entries, seen)
	if len(fresh) != len(entries) {
		t.Fatalf("expected all entries to be new, got %d", len(fresh))
	}
	if !latest.Equal(base) {
		t.Fatalf("expected latest timestamp %v, got %v", base, latest)
	}
	for i := 1; i < len(fresh); i++ {
		if fresh[i-1].OccurredAt.After(fresh[i].OccurredAt) {
			t.Fatalf("expected chronological order, entry %d is after entry %d", i-1, i)
		}
	}
	fresh, latest = filterNewStreamEntries(entries, seen)
	if len(fresh) != 0 {
		t.Fatalf("expected duplicates filtered, got %d", len(fresh))
	}
	if !latest.IsZero() {
		t.Fatalf("expected zero timestamp for no new entries, got %v", latest)
	}
}

func TestStreamEntryKeyFallback(t *testing.T) {
	first := dashboard.StreamEntry{Type: dashboard.StreamTypeAlert, OccurredAt: time.Unix(100, 10)}
	second := dashboard.StreamEntry{Type: dashboard.StreamTypeAlert, OccurredAt: time.Unix(100, 20)}
	if streamEntryKey(first) == streamEntryKey(second) {
		t.Fatalf("expected unique keys for entries without ids")
	}
}
