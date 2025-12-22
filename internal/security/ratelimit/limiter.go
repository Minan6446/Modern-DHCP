package ratelimit

import (
	"context"
	"time"
)

// Key identifies a rate limit dimension (tenant/MAC/port).
type Key struct {
	TenantID  string
	MAC       string
	PortID    string
	IPAddress string
}

// Decision represents the limiter outcome.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// Limiter decides whether a packet should proceed.
type Limiter interface {
	Allow(ctx context.Context, key Key) (Decision, error)
}

type noopLimiter struct{}

// NewNoopLimiter returns a limiter that never blocks.
func NewNoopLimiter() Limiter {
	return &noopLimiter{}
}

func (n *noopLimiter) Allow(ctx context.Context, key Key) (Decision, error) {
	return Decision{Allowed: true}, nil
}
