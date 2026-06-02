package ratelimit

import (
	"sync"
	"time"
)

// Bucket is a token bucket compatible with the minimal API used by Modern-DHCP.
type Bucket struct {
	rate     float64
	capacity float64
	tokens   float64
	last     time.Time
	mu       sync.Mutex
}

// NewBucketWithRate returns a bucket that refills at rate tokens/sec up to capacity.
func NewBucketWithRate(rate float64, capacity int64) *Bucket {
	if rate <= 0 {
		rate = 1
	}
	if capacity <= 0 {
		capacity = 1
	}
	now := time.Now()
	return &Bucket{rate: rate, capacity: float64(capacity), tokens: float64(capacity), last: now}
}

// TakeAvailable tries to remove count tokens immediately and returns granted count.
func (b *Bucket) TakeAvailable(count int64) int64 {
	if b == nil || count <= 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.last = now
	}
	need := float64(count)
	if b.tokens < need {
		return 0
	}
	b.tokens -= need
	return count
}
