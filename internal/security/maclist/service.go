package maclist

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const defaultCacheTTL = 5 * time.Minute

// Service provides CRUD and evaluation for MAC lists.
type Service struct {
	repo     Repository
	logger   *zap.Logger
	cache    *redis.Client
	cacheTTL time.Duration
}

// NewService constructs a Service.
func NewService(repo Repository, logger *zap.Logger) *Service {
	return NewServiceWithCache(repo, logger, nil, 0)
}

// NewServiceWithCache constructs a Service with an optional Redis cache.
func NewServiceWithCache(repo Repository, logger *zap.Logger, cache *redis.Client, cacheTTL time.Duration) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	return &Service{repo: repo, logger: logger, cache: cache, cacheTTL: cacheTTL}
}

// List returns all entries for a tenant.
func (s *Service) List(ctx context.Context, tenantID string) ([]Entry, error) {
	return s.repo.List(ctx, strings.TrimSpace(tenantID))
}

// Get fetches a single entry.
func (s *Service) Get(ctx context.Context, tenantID, id string) (*Entry, error) {
	return s.repo.Get(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(id))
}

// Create inserts a new entry.
func (s *Service) Create(ctx context.Context, tenantID string, entry Entry) (*Entry, error) {
	if err := s.validate(entry); err != nil {
		return nil, err
	}
	entry.ID = uuid.NewString()
	entry.MAC = normalizeMAC(entry.MAC)
	if entry.Priority == 0 {
		entry.Priority = 100
	}
	if entry.Action == "" {
		entry.Action = ActionMonitor
	}
	if entry.Type == ListTypeWhitelist && entry.Action == ActionMonitor {
		entry.Action = ActionAllow
	}
	if err := s.repo.Create(ctx, &entry); err != nil {
		return nil, err
	}
	_ = s.OnMacListChanged(ctx, entry.MAC)
	return &entry, nil
}

// Update mutates an existing entry.
func (s *Service) Update(ctx context.Context, tenantID, id string, entry Entry) (*Entry, error) {
	var previousMAC string
	if previous, err := s.repo.Get(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(id)); err == nil && previous != nil {
		previousMAC = previous.MAC
	}
	if err := s.validate(entry); err != nil {
		return nil, err
	}
	entry.ID = strings.TrimSpace(id)
	entry.MAC = normalizeMAC(entry.MAC)
	if entry.Priority == 0 {
		entry.Priority = 100
	}
	if entry.Action == "" {
		entry.Action = ActionMonitor
	}
	if entry.Type == ListTypeWhitelist && entry.Action == ActionMonitor {
		entry.Action = ActionAllow
	}
	if err := s.repo.Update(ctx, &entry); err != nil {
		return nil, err
	}
	if previousMAC != "" {
		_ = s.OnMacListChanged(ctx, previousMAC)
	}
	_ = s.OnMacListChanged(ctx, entry.MAC)
	return &entry, nil
}

// Delete removes an entry.
func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	tenantID = strings.TrimSpace(tenantID)
	id = strings.TrimSpace(id)
	var previousMAC string
	if previous, err := s.repo.Get(ctx, tenantID, id); err == nil && previous != nil {
		previousMAC = previous.MAC
	}
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	if previousMAC != "" {
		_ = s.OnMacListChanged(ctx, previousMAC)
	}
	return nil
}

// Evaluate returns the highest-priority match for a MAC.
func (s *Service) Evaluate(ctx context.Context, tenantID, mac string) (Result, error) {
	mac = normalizeMAC(mac)
	if mac == "" {
		return Result{}, nil
	}
	tenantID = strings.TrimSpace(tenantID)
	cacheKey := cacheKeyForMAC(mac)

	if s.cache != nil {
		cached, err := s.cache.Get(ctx, cacheKey).Result()
		switch {
		case err == nil:
			s.logger.Info("mac list redis cache hit", zap.String("mac", mac), zap.String("cacheKey", cacheKey))
			var cachedResult Result
			uerr := json.Unmarshal([]byte(cached), &cachedResult)
			if uerr == nil {
				return cachedResult, nil
			}
			s.logger.Warn("mac list redis cache unmarshal failed; deleting key", zap.String("mac", mac), zap.String("cacheKey", cacheKey), zap.Error(uerr))
			if derr := s.cache.Del(ctx, cacheKey).Err(); derr != nil {
				s.logger.Warn("mac list redis DEL failed after decode error", zap.String("mac", mac), zap.String("cacheKey", cacheKey), zap.Error(derr))
			}
		case errors.Is(err, redis.Nil):
			s.logger.Info("mac list redis cache miss", zap.String("mac", mac), zap.String("cacheKey", cacheKey))
		default:
			s.logger.Warn("mac list redis GET failed; fallback to db", zap.String("mac", mac), zap.String("cacheKey", cacheKey), zap.Error(err))
		}
	}

	dbStarted := time.Now()
	entry, err := s.repo.FindByMAC(ctx, tenantID, mac)
	s.logger.Info("mac list db query completed", zap.String("mac", mac), zap.Duration("duration", time.Since(dbStarted)))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			result := Result{}
			s.writeCache(ctx, cacheKey, mac, result)
			return result, nil
		}
		return Result{}, err
	}
	result := Result{Entry: entry}
	s.writeCache(ctx, cacheKey, mac, result)
	return result, nil
}

// OnMacListChanged evicts Redis cache for a MAC after list changes.
func (s *Service) OnMacListChanged(ctx context.Context, mac string) error {
	mac = normalizeMAC(mac)
	if mac == "" || s.cache == nil {
		return nil
	}
	cacheKey := cacheKeyForMAC(mac)
	if err := s.cache.Del(ctx, cacheKey).Err(); err != nil {
		s.logger.Error("mac list redis DEL failed", zap.String("mac", mac), zap.String("cacheKey", cacheKey), zap.Error(err))
		return nil
	}
	return nil
}

func (s *Service) writeCache(ctx context.Context, key, mac string, result Result) {
	if s.cache == nil {
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		s.logger.Warn("mac list redis cache marshal failed", zap.String("mac", mac), zap.String("cacheKey", key), zap.Error(err))
		return
	}
	if err := s.cache.Set(ctx, key, payload, s.cacheTTL).Err(); err != nil {
		s.logger.Warn("mac list redis SET failed", zap.String("mac", mac), zap.String("cacheKey", key), zap.Duration("ttl", s.cacheTTL), zap.Error(err))
	}
}

func cacheKeyForMAC(mac string) string {
	return "dhcp:maclist:{" + mac + "}"
}

func (s *Service) validate(entry Entry) error {
	if strings.TrimSpace(entry.MAC) == "" {
		return errors.New("mac list: mac required")
	}
	switch entry.Type {
	case ListTypeWhitelist, ListTypeBlacklist, ListTypeGraylist:
	default:
		return errors.New("mac list: unsupported list type")
	}
	if entry.Action == "" {
		return nil
	}
	action := strings.ToLower(string(entry.Action))
	switch Action(action) {
	case ActionAllow, ActionBlock, ActionMonitor:
		entry.Action = Action(action)
	default:
		return errors.New("mac list: unsupported action")
	}
	return nil
}

// Ensure Service satisfies Evaluator.
var _ Evaluator = (*Service)(nil)
