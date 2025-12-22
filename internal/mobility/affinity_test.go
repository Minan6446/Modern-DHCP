package mobility

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAffinityCacheLookupLifecycle(t *testing.T) {
	cache := NewAffinityCache(25*time.Millisecond, nil, zap.NewNop())
	if cache == nil {
		t.Fatalf("expected cache instance")
	}
	ctx := context.Background()
	entry := Entry{PoolID: "pool-a"}
	cache.Remember(ctx, "Tenant-A", "AA:BB", entry)
	if got, ok := cache.Lookup(ctx, "tenant-a", "aa:bb"); !ok || got.PoolID != "pool-a" {
		t.Fatalf("expected hit before TTL expiration")
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := cache.Lookup(ctx, "tenant-a", "aa:bb"); ok {
		t.Fatalf("expected entry to expire after TTL")
	}
}
