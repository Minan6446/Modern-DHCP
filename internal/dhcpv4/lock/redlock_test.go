package lock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeLockClient struct {
	mu    sync.Mutex
	locks map[string]fakeLockEntry
}

type fakeLockEntry struct {
	token  string
	expiry time.Time
}

func (f *fakeLockClient) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locks == nil {
		f.locks = map[string]fakeLockEntry{}
	}
	now := time.Now()
	if existing, ok := f.locks[key]; ok && existing.expiry.After(now) {
		return false, nil
	}
	token, _ := value.(string)
	f.locks[key] = fakeLockEntry{token: token, expiry: now.Add(expiration)}
	return true, nil
}

func (f *fakeLockClient) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	if len(keys) != 1 || len(args) != 1 {
		return nil, errors.New("invalid eval arguments")
	}
	key := keys[0]
	token, _ := args[0].(string)
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.locks[key]
	if !ok {
		return int64(0), nil
	}
	if entry.token != token {
		return int64(0), nil
	}
	delete(f.locks, key)
	return int64(1), nil
}

func TestManagerConcurrentAcquire1000(t *testing.T) {
	mgr := &Manager{client: &fakeLockClient{locks: map[string]fakeLockEntry{}}}
	if mgr == nil {
		t.Fatalf("expected lock manager")
	}

	var (
		inCritical int64
		maxSeen    int64
	)
	workers := 1000
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			guard, err := mgr.Acquire(ctx, "dhcpv4:alloc:pool:default", 5*time.Second)
			if err != nil {
				t.Errorf("acquire lock: %v", err)
				return
			}
			cur := atomic.AddInt64(&inCritical, 1)
			for {
				old := atomic.LoadInt64(&maxSeen)
				if cur <= old || atomic.CompareAndSwapInt64(&maxSeen, old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Microsecond)
			atomic.AddInt64(&inCritical, -1)
			if err := guard.Unlock(context.Background()); err != nil && err != errLockNotAcquired {
				t.Errorf("unlock lock: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt64(&maxSeen); got > 1 {
		t.Fatalf("lock broken, concurrent holders=%d", got)
	}
}
