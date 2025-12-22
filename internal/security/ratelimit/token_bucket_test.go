package ratelimit

import (
	"context"
	"testing"
	"time"
)

type fakeClock struct {
	current time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{current: time.Unix(0, 0)}
}

func (f *fakeClock) Now() time.Time {
	return f.current
}

func (f *fakeClock) Advance(d time.Duration) {
	f.current = f.current.Add(d)
}

func TestTokenBucketLimiterEnforcesRate(t *testing.T) {
	clock := newFakeClock()
	limiter := NewTokenBucketLimiter(Options{
		Default: Profile{PerMacPPS: 1, Burst: 1},
		Clock:   clock.Now,
	})

	key := Key{TenantID: "tenant", MAC: "aa:bb", PortID: "gi1/0/1"}

	decision, err := limiter.Allow(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("first packet should be allowed")
	}

	decision, err = limiter.Allow(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("second packet should be throttled")
	}

	clock.Advance(time.Second)
	decision, err = limiter.Allow(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("token bucket should refill after 1s")
	}
}

func TestTokenBucketLimiterOverrides(t *testing.T) {
	clock := newFakeClock()
	limiter := NewTokenBucketLimiter(Options{
		Default: Profile{PerMacPPS: 1, Burst: 1},
		Overrides: map[string]Profile{
			"premium": {PerPortPPS: 5, Burst: 5},
		},
		Clock: clock.Now,
	})

	key := Key{TenantID: "premium", MAC: "aa", PortID: "iface"}

	for i := 0; i < 5; i++ {
		decision, err := limiter.Allow(context.Background(), key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("override should allow burst, iteration %d", i)
		}
	}

	decision, err := limiter.Allow(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("burst should be exhausted")
	}
}

func TestTokenBucketLimiterPerIP(t *testing.T) {
	clock := newFakeClock()
	limiter := NewTokenBucketLimiter(Options{
		Default: Profile{PerIPPPS: 1, Burst: 1},
		Clock:   clock.Now,
	})
	key := Key{TenantID: "t1", MAC: "aa", PortID: "gi", IPAddress: "10.0.0.5"}
	if decision, err := limiter.Allow(context.Background(), key); err != nil || !decision.Allowed {
		t.Fatalf("first packet should pass, decision=%+v err=%v", decision, err)
	}
	decision, err := limiter.Allow(context.Background(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("per-ip limit should throttle second packet")
	}
	clock.Advance(time.Second)
	decision, err = limiter.Allow(context.Background(), key)
	if err != nil || !decision.Allowed {
		t.Fatalf("token should refill after 1s, decision=%+v err=%v", decision, err)
	}
	key.IPAddress = "10.0.0.6"
	decision, err = limiter.Allow(context.Background(), key)
	if err != nil || !decision.Allowed {
		t.Fatalf("new ip should have independent bucket, decision=%+v err=%v", decision, err)
	}
}
