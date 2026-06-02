package cache

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Store is a minimal cache interface supporting tiered implementations.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	GetJSON(ctx context.Context, key string, dst any) (bool, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
}

// LayeredCache provides L1 in-memory caching with optional Redis L2.
type LayeredCache struct {
	l1        *memoryCache
	l2        redis.UniversalClient
	defaultTT time.Duration
	prefix    string
	logger    *zap.Logger
}

// Options configures LayeredCache behavior.
type Options struct {
	DefaultTTL time.Duration
	Prefix     string
	MaxItems   int
	Logger     *zap.Logger
}

// NewLayeredCache builds a layered cache with optional redis backend.
func NewLayeredCache(redisClient redis.UniversalClient, opts Options) *LayeredCache {
	ttl := opts.DefaultTTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	maxItems := opts.MaxItems
	if maxItems <= 0 {
		maxItems = 2048
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LayeredCache{
		l1:        newMemoryCache(maxItems),
		l2:        redisClient,
		defaultTT: ttl,
		prefix:    opts.Prefix,
		logger:    logger,
	}
}

func (c *LayeredCache) namespaced(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + ":" + key
}

// Get returns a value from cache. It populates L1 from L2 on hit.
func (c *LayeredCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	if val, ok := c.l1.get(key); ok {
		return val, true, nil
	}
	if c.l2 == nil {
		return nil, false, nil
	}
	namespaced := c.namespaced(key)
	res, err := c.l2.Get(ctx, namespaced).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		c.logger.Warn("cache: redis get failed", zap.String("key", namespaced), zap.Error(err))
		return nil, false, err
	}
	c.l1.set(key, res, c.defaultTT)
	return res, true, nil
}

// Set writes through to L1 and L2.
func (c *LayeredCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if c == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = c.defaultTT
	}
	c.l1.set(key, value, ttl)
	if c.l2 == nil {
		return nil
	}
	namespaced := c.namespaced(key)
	if err := c.l2.Set(ctx, namespaced, value, ttl).Err(); err != nil {
		c.logger.Warn("cache: redis set failed", zap.String("key", namespaced), zap.Error(err))
		return err
	}
	return nil
}

// Delete evicts from L1 and L2.
func (c *LayeredCache) Delete(ctx context.Context, key string) error {
	if c == nil {
		return nil
	}
	c.l1.delete(key)
	if c.l2 == nil {
		return nil
	}
	namespaced := c.namespaced(key)
	if err := c.l2.Del(ctx, namespaced).Err(); err != nil {
		if err == redis.Nil {
			return nil
		}
		c.logger.Warn("cache: redis delete failed", zap.String("key", namespaced), zap.Error(err))
		return err
	}
	return nil
}

// DeletePrefix evicts keys that share the provided prefix.
func (c *LayeredCache) DeletePrefix(ctx context.Context, prefix string) error {
	if c == nil {
		return nil
	}
	c.l1.deletePrefix(prefix)
	if c.l2 == nil {
		return nil
	}
	match := c.namespaced(prefix) + "*"
	iter := c.l2.Scan(ctx, 0, match, 200)
	for {
		keys, cursor, err := iter.Result()
		if err != nil {
			c.logger.Warn("cache: redis scan failed", zap.String("match", match), zap.Error(err))
			return err
		}
		if len(keys) > 0 {
			if err := c.l2.Del(ctx, keys...).Err(); err != nil {
				c.logger.Warn("cache: redis deleteprefix failed", zap.String("match", match), zap.Error(err))
				return err
			}
		}
		if cursor == 0 {
			break
		}
		iter = c.l2.Scan(ctx, cursor, match, 200)
	}
	return nil
}

// GetJSON unmarshals JSON payload into dst if present.
func (c *LayeredCache) GetJSON(ctx context.Context, key string, dst any) (bool, error) {
	data, ok, err := c.Get(ctx, key)
	if err != nil || !ok {
		return ok, err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		c.logger.Warn("cache: json unmarshal failed", zap.String("key", key), zap.Error(err))
		return false, err
	}
	return true, nil
}

// SetJSON marshals and stores JSON payload.
func (c *LayeredCache) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, payload, ttl)
}

// HashKey helps create deterministic cache keys.
func HashKey(parts ...string) string {
	h := sha1.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte("|"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// memoryCache is a simple TTL map for L1.
type memoryCache struct {
	mu       sync.RWMutex
	entries  map[string]l1entry
	maxItems int
}

type l1entry struct {
	value  []byte
	expiry time.Time
}

func newMemoryCache(maxItems int) *memoryCache {
	return &memoryCache{entries: make(map[string]l1entry), maxItems: maxItems}
}

func (m *memoryCache) get(key string) ([]byte, bool) {
	m.mu.RLock()
	entry, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok || time.Now().After(entry.expiry) {
		if ok {
			m.delete(key)
		}
		return nil, false
	}
	return entry.value, true
}

func (m *memoryCache) set(key string, value []byte, ttl time.Duration) {
	if ttl <= 0 {
		ttl = time.Minute
	}
	m.mu.Lock()
	if len(m.entries) >= m.maxItems {
		// naive eviction: remove one arbitrary entry
		for k := range m.entries {
			delete(m.entries, k)
			break
		}
	}
	m.entries[key] = l1entry{value: value, expiry: time.Now().Add(ttl)}
	m.mu.Unlock()
}

func (m *memoryCache) delete(key string) {
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
}

func (m *memoryCache) deletePrefix(prefix string) {
	m.mu.Lock()
	for k := range m.entries {
		if strings.HasPrefix(k, prefix) {
			delete(m.entries, k)
		}
	}
	m.mu.Unlock()
}
