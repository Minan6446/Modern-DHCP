package detector

import (
	"context"
	"testing"
	"time"
)

func TestDetectorBlocksRogueServer(t *testing.T) {
	det := NewSimpleDetector(Config{
		RogueServerAllowlist:   []string{"10.0.0.1"},
		BlockDuration:          time.Minute,
		DiscoverToRequestRatio: 0.5,
	})

	verdict := det.Observe(context.Background(), Event{
		TenantID: "tenant-a",
		MAC:      "00:11:22:33:44:55",
		GIAddr:   "10.0.0.2",
	})

	if !verdict.Block || verdict.Reason != "rogue_server" {
		t.Fatalf("expected rogue server block, got %+v", verdict)
	}
}

func TestDetectorBlocksStarvationPattern(t *testing.T) {
	det := NewSimpleDetector(Config{
		DiscoverBurstThreshold: 3,
		DiscoverToRequestRatio: 0.5,
		BlockDuration:          time.Minute,
	})

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		verdict := det.Observe(ctx, Event{TenantID: "t1", MAC: "aa:bb", MessageType: "discover"})
		if verdict.Block {
			t.Fatalf("unexpected block on iteration %d: %+v", i, verdict)
		}
	}

	verdict := det.Observe(ctx, Event{TenantID: "t1", MAC: "aa:bb", MessageType: "discover"})
	if !verdict.Block || verdict.Reason != "starvation_pattern" {
		t.Fatalf("expected starvation block, got %+v", verdict)
	}

	verdict = det.Observe(ctx, Event{TenantID: "t1", MAC: "aa:bb", MessageType: "request"})
	if !verdict.Block || verdict.Reason != "starvation_pattern" {
		t.Fatalf("expected continued block, got %+v", verdict)
	}
}

func TestDetectorBlocksDeclineSpike(t *testing.T) {
	det := NewSimpleDetector(Config{
		DeclineSpikeThreshold: 2,
		BlockDuration:         time.Minute,
	})
	ctx := context.Background()

	v1 := det.Observe(ctx, Event{TenantID: "acme", MAC: "00:aa", MessageType: "decline"})
	if v1.Block {
		t.Fatalf("first decline should not block: %+v", v1)
	}

	v2 := det.Observe(ctx, Event{TenantID: "acme", MAC: "00:aa", MessageType: "decline"})
	if !v2.Block || v2.Reason != "decline_spike" {
		t.Fatalf("expected block on second decline spike: %+v", v2)
	}
}

func TestDetectorBlocksMacDrift(t *testing.T) {
	det := NewSimpleDetector(Config{
		MACDriftThreshold: 1,
		BlockDuration:     time.Minute,
	})
	ctx := context.Background()
	first := det.Observe(ctx, Event{TenantID: "acme", MAC: "00:aa", PortID: "gi1/0/1"})
	if first.Block {
		t.Fatalf("first port should pass: %+v", first)
	}
	second := det.Observe(ctx, Event{TenantID: "acme", MAC: "00:aa", PortID: "gi1/0/2"})
	if !second.Block || second.Reason != "mac_drift" {
		t.Fatalf("expected mac drift block: %+v", second)
	}
}

func TestDetectorBlocksOption82Mismatch(t *testing.T) {
	det := NewSimpleDetector(Config{
		Option82MismatchTolerance: 1,
		BlockDuration:             time.Minute,
	})
	ctx := context.Background()
	_ = det.Observe(ctx, Event{TenantID: "acme", MAC: "00:aa", PortID: "gi1/0/1"})
	second := det.Observe(ctx, Event{TenantID: "acme", MAC: "00:aa", PortID: "gi1/0/2"})
	if second.Block {
		t.Fatalf("first mismatch should be tolerated: %+v", second)
	}
	third := det.Observe(ctx, Event{TenantID: "acme", MAC: "00:aa", PortID: "gi1/0/1"})
	if !third.Block || third.Reason != "option82_mismatch" {
		t.Fatalf("expected option82 mismatch block: %+v", third)
	}
}
