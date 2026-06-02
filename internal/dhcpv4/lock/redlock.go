package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var errLockNotAcquired = errors.New("dhcpv4 lock: not acquired")

const releaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`

// Guard tracks an acquired distributed lock.
type Guard struct {
	client redisLockClient
	key    string
	token  string
}

// Unlock releases the lock only when the owner token matches.
func (g *Guard) Unlock(ctx context.Context) error {
	if g == nil || g.client == nil || g.key == "" || g.token == "" {
		return nil
	}
	result, err := g.client.Eval(ctx, releaseScript, []string{g.key}, g.token)
	if err != nil {
		return err
	}
	if n, ok := result.(int64); ok && n == 0 {
		return errLockNotAcquired
	}
	return nil
}

// Manager provides RedLock-style lock primitives on top of Redis.
type Manager struct {
	client redisLockClient
}

type redisLockClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error)
	Eval(ctx context.Context, script string, keys []string, args ...any) (any, error)
}

type redisLockAdapter struct {
	client redis.UniversalClient
}

func (a *redisLockAdapter) SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error) {
	return a.client.SetNX(ctx, key, value, expiration).Result()
}

func (a *redisLockAdapter) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return a.client.Eval(ctx, script, keys, args...).Result()
}

// NewManager builds a lock manager.
func NewManager(client redis.UniversalClient) *Manager {
	if client == nil {
		return nil
	}
	return &Manager{client: &redisLockAdapter{client: client}}
}

// Acquire attempts lock acquisition with retry until context timeout.
func (m *Manager) Acquire(ctx context.Context, key string, ttl time.Duration) (*Guard, error) {
	if m == nil || m.client == nil {
		return nil, nil
	}
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	retry := 25 * time.Millisecond
	for {
		ok, err := m.client.SetNX(ctx, key, token, ttl)
		if err != nil {
			return nil, err
		}
		if ok {
			return &Guard{client: m.client, key: key, token: token}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retry):
		}
	}
}

func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
