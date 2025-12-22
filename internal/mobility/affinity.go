package mobility

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/metrics"
)

// Coordinates capture the contextual hints observed when a mobility decision was recorded.
type Coordinates struct {
	AnchorID      string
	AccessPointID string
	ControllerID  string
	GeoZone       string
	Location      string
	VLANID        int
}

// Entry represents a cached affinity decision for a device.
type Entry struct {
	PoolID      string
	Coordinates Coordinates
	storedAt    time.Time
}

// Cache exposes lookup/update semantics for mobility affinity decisions.
type Cache interface {
	Lookup(ctx context.Context, tenantID, identifier string) (Entry, bool)
	Remember(ctx context.Context, tenantID, identifier string, entry Entry)
}

// AffinityCache provides an in-memory TTL cache that can later be swapped with Redis.
type AffinityCache struct {
	ttl     time.Duration
	metrics *metrics.Collector
	logger  *zap.Logger
	mu      sync.RWMutex
	entries map[string]Entry
}

// NewAffinityCache constructs a cache if ttl > 0; otherwise returns nil to signal disabled mode.
func NewAffinityCache(ttl time.Duration, metricsCollector *metrics.Collector, logger *zap.Logger) *AffinityCache {
	if ttl <= 0 {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AffinityCache{
		ttl:     ttl,
		metrics: metricsCollector,
		logger:  logger,
		entries: make(map[string]Entry),
	}
}

// Lookup attempts to return a cached entry; expired entries are evicted eagerly.
func (c *AffinityCache) Lookup(_ context.Context, tenantID, identifier string) (Entry, bool) {
	var empty Entry
	if c == nil {
		return empty, false
	}
	key := c.cacheKey(tenantID, identifier)
	if key == "" {
		return empty, false
	}
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		c.observe(tenantID, "miss")
		return empty, false
	}
	if entry.storedAt.Add(c.ttl).Before(now) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		c.observe(tenantID, "miss")
		return empty, false
	}
	c.observe(tenantID, "hit")
	return entry, true
}

// Remember stores or refreshes an entry until the configured TTL elapses.
func (c *AffinityCache) Remember(_ context.Context, tenantID, identifier string, entry Entry) {
	if c == nil || entry.PoolID == "" {
		return
	}
	key := c.cacheKey(tenantID, identifier)
	if key == "" {
		return
	}
	entry.storedAt = time.Now()
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
}

func (c *AffinityCache) cacheKey(tenantID, identifier string) string {
	tenantID = strings.TrimSpace(strings.ToLower(tenantID))
	identifier = strings.TrimSpace(strings.ToLower(identifier))
	if tenantID == "" || identifier == "" {
		return ""
	}
	return tenantID + "|" + identifier
}

func (c *AffinityCache) observe(tenantID, result string) {
	if c == nil || c.metrics == nil || c.metrics.MobilityAffinityHits == nil {
		return
	}
	if tenantID == "" {
		tenantID = "unknown"
	}
	c.metrics.MobilityAffinityHits.WithLabelValues(tenantID, result).Inc()
}
