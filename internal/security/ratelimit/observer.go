package ratelimit

import "time"

// Hit captures metadata for a rate-limit rejection emitted by the guard.
type Hit struct {
	TenantID   string
	MAC        string
	PortID     string
	IPAddress  string
	RetryAfter time.Duration
	OccurredAt time.Time
}

// Observer records rate-limit hits for downstream processing.
type Observer interface {
	Record(hit Hit)
}
