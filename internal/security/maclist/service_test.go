package maclist

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type stubRepo struct {
	findByMACCalls int
	entry          *Entry
	err            error
}

func (s *stubRepo) List(ctx context.Context, tenantID string) ([]Entry, error) {
	return nil, nil
}

func (s *stubRepo) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	return nil, ErrNotFound
}

func (s *stubRepo) FindByMAC(ctx context.Context, tenantID, mac string) (*Entry, error) {
	s.findByMACCalls++
	if s.err != nil {
		return nil, s.err
	}
	if s.entry == nil {
		return nil, ErrNotFound
	}
	copyEntry := *s.entry
	return &copyEntry, nil
}

func (s *stubRepo) Create(ctx context.Context, entry *Entry) error {
	return nil
}

func (s *stubRepo) Update(ctx context.Context, entry *Entry) error {
	return nil
}

func (s *stubRepo) Delete(ctx context.Context, tenantID, id string) error {
	return nil
}

func TestEvaluateRedisCacheHit(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	mac := "f0:8f:4c:5f:b6:42"
	key := "dhcp:maclist:{" + mac + "}"
	cached := Result{Entry: &Entry{ID: "cached-entry", MAC: mac, Type: ListTypeWhitelist, Action: ActionAllow, Priority: 10}}
	payload, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("marshal cache payload: %v", err)
	}
	mini.Set(key, string(payload))

	repo := &stubRepo{entry: &Entry{ID: "db-entry", MAC: mac}}
	svc := NewServiceWithCache(repo, zap.NewNop(), client, time.Minute)

	result, err := svc.Evaluate(context.Background(), "tenant-a", mac)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if result.Entry == nil || result.Entry.ID != "cached-entry" {
		t.Fatalf("expected cached entry, got %+v", result.Entry)
	}
	if repo.findByMACCalls != 0 {
		t.Fatalf("expected no db calls on cache hit, got %d", repo.findByMACCalls)
	}
}

func TestEvaluateRedisCacheMiss(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	mac := "f0:8f:4c:5f:b6:42"
	repo := &stubRepo{entry: &Entry{ID: "db-entry", MAC: mac, Type: ListTypeBlacklist, Action: ActionBlock, Priority: 1}}
	svc := NewServiceWithCache(repo, zap.NewNop(), client, 2*time.Minute)

	result, err := svc.Evaluate(context.Background(), "tenant-a", mac)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if result.Entry == nil || result.Entry.ID != "db-entry" {
		t.Fatalf("expected db entry, got %+v", result.Entry)
	}
	if repo.findByMACCalls != 1 {
		t.Fatalf("expected one db call, got %d", repo.findByMACCalls)
	}
	key := "dhcp:maclist:{" + mac + "}"
	if !mini.Exists(key) {
		t.Fatalf("expected cache key %s to be written", key)
	}
}

func TestEvaluateRedisFailureFallback(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:0",
		DialTimeout:  20 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })

	mac := "f0:8f:4c:5f:b6:42"
	repo := &stubRepo{entry: &Entry{ID: "db-entry", MAC: mac, Type: ListTypeWhitelist, Action: ActionAllow}}
	svc := NewServiceWithCache(repo, zap.NewNop(), client, time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result, err := svc.Evaluate(ctx, "tenant-a", mac)
	if err != nil {
		t.Fatalf("evaluate should fallback to db, got error: %v", err)
	}
	if result.Entry == nil || result.Entry.ID != "db-entry" {
		t.Fatalf("expected db fallback result, got %+v", result.Entry)
	}
	if repo.findByMACCalls != 1 {
		t.Fatalf("expected one db call during fallback, got %d", repo.findByMACCalls)
	}
}

func TestOnMacListChangedInvalidatesCache(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	mac := "f0:8f:4c:5f:b6:42"
	key := "dhcp:maclist:{" + mac + "}"
	mini.Set(key, `{"Entry":{"ID":"cached"}}`)

	svc := NewServiceWithCache(&stubRepo{}, zap.NewNop(), client, time.Minute)
	if err := svc.OnMacListChanged(context.Background(), mac); err != nil {
		t.Fatalf("OnMacListChanged returned unexpected error: %v", err)
	}
	if mini.Exists(key) {
		t.Fatalf("expected cache key %s to be deleted", key)
	}
}

func TestEvaluateDatabaseErrorSkipsCacheWrite(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	repo := &stubRepo{err: errors.New("db unavailable")}
	svc := NewServiceWithCache(repo, zap.NewNop(), client, time.Minute)
	mac := "f0:8f:4c:5f:b6:42"

	_, err := svc.Evaluate(context.Background(), "tenant-a", mac)
	if err == nil {
		t.Fatalf("expected db error")
	}
	key := "dhcp:maclist:{" + mac + "}"
	if mini.Exists(key) {
		t.Fatalf("cache should not be written when db query fails")
	}
}
