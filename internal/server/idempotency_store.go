package server

import (
	"sync"
	"time"
)

type idempotencyEntry struct {
	expiresAt time.Time
	status    int
	payload   []byte
}

type idempotencyStore struct {
	mu    sync.Mutex
	items map[string]idempotencyEntry
}

func newIdempotencyStore() *idempotencyStore {
	return &idempotencyStore{items: make(map[string]idempotencyEntry)}
}

func (s *idempotencyStore) get(key string) (idempotencyEntry, bool) {
	if s == nil || key == "" {
		return idempotencyEntry{}, false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[key]
	if !ok {
		return idempotencyEntry{}, false
	}
	if now.After(entry.expiresAt) {
		delete(s.items, key)
		return idempotencyEntry{}, false
	}
	return entry, true
}

func (s *idempotencyStore) set(key string, status int, payload []byte, ttl time.Duration) {
	if s == nil || key == "" {
		return
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	s.mu.Lock()
	s.items[key] = idempotencyEntry{status: status, payload: append([]byte(nil), payload...), expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
}

var poolImportIdempotencyStore = newIdempotencyStore()
