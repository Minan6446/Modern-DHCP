package ratelimit

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Profile describes throttling parameters.
type Profile struct {
	PerMacPPS  int
	PerPortPPS int
	PerIPPPS   int
	Burst      int
}

// Options configures the token bucket limiter.
type Options struct {
	Default   Profile
	Overrides map[string]Profile
	Clock     func() time.Time
}

type tokenBucketLimiter struct {
	opts    Options
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens   float64
	lastTime time.Time
}

// NewTokenBucketLimiter builds an in-memory token bucket limiter.
func NewTokenBucketLimiter(opts Options) Limiter {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	return &tokenBucketLimiter{opts: opts, buckets: make(map[string]*bucket)}
}

func (l *tokenBucketLimiter) Allow(ctx context.Context, key Key) (Decision, error) {
	profile := l.profileFor(key.TenantID)
	now := l.opts.Clock()

	l.mu.Lock()
	defer l.mu.Unlock()

	if allowed, retry := l.consume(now, "mac", key.TenantID, profile.PerMacPPS, profile.Burst, key.MAC); !allowed {
		return Decision{Allowed: false, RetryAfter: retry}, nil
	}
	if allowed, retry := l.consume(now, "port", key.TenantID, profile.PerPortPPS, profile.Burst, key.PortID); !allowed {
		return Decision{Allowed: false, RetryAfter: retry}, nil
	}
	if allowed, retry := l.consume(now, "ip", key.TenantID, profile.PerIPPPS, profile.Burst, key.IPAddress); !allowed {
		return Decision{Allowed: false, RetryAfter: retry}, nil
	}

	return Decision{Allowed: true}, nil
}

func (l *tokenBucketLimiter) profileFor(tenant string) Profile {
	if l.opts.Overrides != nil {
		if p, ok := l.opts.Overrides[tenant]; ok {
			return l.normalizeProfile(p)
		}
	}
	return l.normalizeProfile(l.opts.Default)
}

func (l *tokenBucketLimiter) normalizeProfile(p Profile) Profile {
	if p.PerMacPPS < 0 {
		p.PerMacPPS = 0
	}
	if p.PerPortPPS < 0 {
		p.PerPortPPS = 0
	}
	if p.PerIPPPS < 0 {
		p.PerIPPPS = 0
	}
	if p.Burst < 0 {
		p.Burst = 0
	}
	return p
}

func (l *tokenBucketLimiter) consume(now time.Time, dimension string, tenant string, limit int, burst int, value string) (bool, time.Duration) {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return true, 0
	}
	if burst <= 0 {
		burst = limit
	}
	key := l.dimensionKey(dimension, tenant, value)
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: float64(burst), lastTime: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += float64(limit) * elapsed
	if b.tokens > float64(burst) {
		b.tokens = float64(burst)
	}
	b.lastTime = now
	if b.tokens >= 1 {
		b.tokens -= 1
		return true, 0
	}
	deficit := 1 - b.tokens
	perSecond := float64(limit)
	retryAfter := time.Duration((deficit / perSecond) * float64(time.Second))
	if retryAfter < 10*time.Millisecond {
		retryAfter = 10 * time.Millisecond
	}
	return false, retryAfter
}

func (l *tokenBucketLimiter) dimensionKey(dimension, tenant, value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(len(dimension) + len(tenant) + len(normalized) + 2)
	builder.WriteString(dimension)
	builder.WriteByte('|')
	builder.WriteString(tenant)
	builder.WriteByte('|')
	builder.WriteString(normalized)
	return builder.String()
}
