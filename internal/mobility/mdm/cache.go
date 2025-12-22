package mdm

import (
	"strings"
	"sync"
	"time"
)

type cacheEntry struct {
	record   ComplianceRecord
	storedAt time.Time
}

// Cache stores compliance records with a TTL to ensure stale data is evicted automatically.
type Cache struct {
	ttl     time.Duration
	mu      sync.RWMutex
	records map[string]cacheEntry
}

// NewCache creates a TTL-backed cache; if ttl <= 0 a sane default is applied.
func NewCache(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &Cache{ttl: ttl, records: make(map[string]cacheEntry)}
}

// Remember inserts or refreshes the given record.
func (c *Cache) Remember(record ComplianceRecord) {
	if c == nil {
		return
	}
	key := cacheKey(record.TenantID, record.DeviceID)
	if key == "" {
		return
	}
	c.mu.Lock()
	c.records[key] = cacheEntry{record: record, storedAt: time.Now()}
	c.mu.Unlock()
}

// Lookup returns the latest compliance record if available and not expired.
func (c *Cache) Lookup(tenantID, deviceID string) (ComplianceRecord, bool) {
	var empty ComplianceRecord
	if c == nil {
		return empty, false
	}
	key := cacheKey(tenantID, deviceID)
	if key == "" {
		return empty, false
	}
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.records[key]
	c.mu.RUnlock()
	if !ok {
		return empty, false
	}
	if entry.record.ExpiresAt.IsZero() {
		entry.record.ExpiresAt = entry.storedAt.Add(c.ttl)
	}
	expiry := entry.record.ExpiresAt
	if expiry.Before(now) {
		c.mu.Lock()
		delete(c.records, key)
		c.mu.Unlock()
		return empty, false
	}
	return entry.record, true
}

func cacheKey(tenantID, deviceID string) string {
	tenantID = strings.TrimSpace(strings.ToLower(tenantID))
	deviceID = strings.TrimSpace(strings.ToLower(deviceID))
	if tenantID == "" || deviceID == "" {
		return ""
	}
	return tenantID + "|" + deviceID
}
