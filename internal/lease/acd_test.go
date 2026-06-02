package lease

import (
	"context"
	"encoding/json"
	"net/netip"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

type testACDProber struct {
	arpAlive  bool
	icmpAlive bool
}

func (p *testACDProber) ProbeARP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error) {
	return p.arpAlive, time.Millisecond, nil
}

func (p *testACDProber) ProbeICMP(ctx context.Context, addr netip.Addr) (bool, time.Duration, error) {
	return p.icmpAlive, time.Millisecond, nil
}

type testCacheStore struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func (c *testCacheStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.m[key]
	if !ok {
		return nil, false, nil
	}
	copyV := append([]byte(nil), v...)
	return copyV, true, nil
}

func (c *testCacheStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string][]byte{}
	}
	c.m[key] = append([]byte(nil), value...)
	return nil
}

func (c *testCacheStore) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, key)
	return nil
}

func (c *testCacheStore) DeletePrefix(ctx context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.m {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.m, k)
		}
	}
	return nil
}

func (c *testCacheStore) GetJSON(ctx context.Context, key string, dst any) (bool, error) {
	data, ok, err := c.Get(ctx, key)
	if err != nil || !ok {
		return ok, err
	}
	return true, json.Unmarshal(data, dst)
}

func (c *testCacheStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, data, ttl)
}

func TestACDCheckCandidateConflict(t *testing.T) {
	repo := &MySQLRepository{cache: &testCacheStore{}, cacheTTL: 30 * time.Second}
	svc := &Service{
		repo:           repo,
		logger:         zap.NewNop(),
		conflictProber: &testACDProber{arpAlive: true},
		conflictCfg: config.ConflictPreventionConfig{
			Enabled:     true,
			ARPTimeout:  time.Second,
			ARPRetries:  1,
			ICMPRetries: 1,
		},
	}
	ok, err := svc.acdCheckCandidate(context.Background(), "pool-a", "10.0.0.10")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ok {
		t.Fatalf("expected candidate rejected due to ARP conflict")
	}
}

func TestACDCheckCandidateCacheHit(t *testing.T) {
	store := &testCacheStore{}
	repo := &MySQLRepository{cache: store, cacheTTL: 30 * time.Second}
	svc := &Service{
		repo:           repo,
		logger:         zap.NewNop(),
		conflictProber: &testACDProber{arpAlive: false, icmpAlive: false},
		conflictCfg: config.ConflictPreventionConfig{
			Enabled: true,
		},
	}
	key := svc.acdCacheKey("pool-a", "10.0.0.11")
	if err := store.SetJSON(context.Background(), key, acdProbeResult{Conflict: true, Method: "icmp", CheckedAt: time.Now().UTC()}, 30*time.Second); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	ok, err := svc.acdCheckCandidate(context.Background(), "pool-a", "10.0.0.11")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ok {
		t.Fatalf("expected cache conflict to reject candidate")
	}
}
