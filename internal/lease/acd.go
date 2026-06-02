package lease

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/cache"
	"modern-dhcp/internal/config"
)

type acdProbeResult struct {
	Conflict  bool      `json:"conflict"`
	Method    string    `json:"method"`
	CheckedAt time.Time `json:"checkedAt"`
}

type acdEffectiveConfig struct {
	Enabled     bool
	ARPTimeout  time.Duration
	ARPRetries  int
	ICMPRetries int
	CacheTTL    time.Duration
	ConflictTTL time.Duration
}

func (s *Service) acdCacheStore() (cache.Store, time.Duration) {
	repo, ok := s.repo.(*MySQLRepository)
	if !ok || repo == nil || repo.cache == nil {
		return nil, 0
	}
	return repo.cache, repo.cacheTTL
}

func (s *Service) acdCacheKey(poolID, ip string) string {
	return fmt.Sprintf("dhcpv4:acd:%s:%s", strings.TrimSpace(poolID), strings.TrimSpace(ip))
}

func (s *Service) acdPoolConfig(poolID string) (config.ConflictPreventionPoolConfig, bool) {
	if s.conflictCfg.Pools == nil {
		return config.ConflictPreventionPoolConfig{}, false
	}
	cfg, ok := s.conflictCfg.Pools[strings.TrimSpace(poolID)]
	return cfg, ok
}

func (s *Service) acdEffectiveConfig(poolID string) acdEffectiveConfig {
	effective := acdEffectiveConfig{
		Enabled:     s.conflictCfg.Enabled,
		ARPTimeout:  s.conflictProbeTimeout(),
		ARPRetries:  2,
		ICMPRetries: 2,
		CacheTTL:    30 * time.Second,
		ConflictTTL: 2 * time.Minute,
	}
	if s.conflictCfg.ARPTimeout > 0 {
		effective.ARPTimeout = s.conflictCfg.ARPTimeout
	}
	if s.conflictCfg.ARPRetries > 0 {
		effective.ARPRetries = s.conflictCfg.ARPRetries
	}
	if s.conflictCfg.ICMPRetries > 0 {
		effective.ICMPRetries = s.conflictCfg.ICMPRetries
	}
	if s.conflictCfg.CacheTTL > 0 {
		effective.CacheTTL = s.conflictCfg.CacheTTL
	}
	if s.conflictCfg.ConflictTTL > 0 {
		effective.ConflictTTL = s.conflictCfg.ConflictTTL
	}
	if override, ok := s.acdPoolConfig(poolID); ok {
		effective.Enabled = override.Enabled
		if override.ARPTimeout > 0 {
			effective.ARPTimeout = override.ARPTimeout
		}
		if override.ARPRetries > 0 {
			effective.ARPRetries = override.ARPRetries
		}
		if override.ICMPRetries > 0 {
			effective.ICMPRetries = override.ICMPRetries
		}
	}
	return effective
}

func (s *Service) acdProbeFromCache(ctx context.Context, poolID, ip string) (acdProbeResult, bool) {
	store, _ := s.acdCacheStore()
	if store == nil {
		return acdProbeResult{}, false
	}
	key := s.acdCacheKey(poolID, ip)
	var result acdProbeResult
	ok, err := store.GetJSON(ctx, key, &result)
	if err != nil || !ok {
		return acdProbeResult{}, false
	}
	return result, true
}

func (s *Service) acdStoreResultAsync(poolID, ip string, result acdProbeResult, ttl time.Duration) {
	store, fallbackTTL := s.acdCacheStore()
	if store == nil {
		return
	}
	if ttl <= 0 {
		ttl = fallbackTTL
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	key := s.acdCacheKey(poolID, ip)
	go func() {
		_ = store.SetJSON(context.Background(), key, result, ttl)
	}()
}

func (s *Service) acdStoreConflictSync(ctx context.Context, poolID, ip string, result acdProbeResult, ttl time.Duration) error {
	store, fallbackTTL := s.acdCacheStore()
	if store == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = fallbackTTL
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return store.SetJSON(ctx, s.acdCacheKey(poolID, ip), result, ttl)
}

func (s *Service) acdProbeAddress(ctx context.Context, poolID, ip string) (acdProbeResult, error) {
	result := acdProbeResult{Conflict: false, CheckedAt: time.Now().UTC()}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.Is4() {
		return result, nil
	}
	addr = addr.Unmap()
	if !addr.IsPrivate() {
		return result, nil
	}
	cfg := s.acdEffectiveConfig(poolID)
	if !cfg.Enabled || s.conflictProber == nil {
		return result, nil
	}
	for i := 0; i < cfg.ARPRetries; i++ {
		probeCtx, cancel := context.WithTimeout(ctx, cfg.ARPTimeout)
		alive, _, probeErr := s.conflictProber.ProbeARP(probeCtx, addr)
		cancel()
		if probeErr != nil && s.logger != nil {
			s.logger.Debug("acd arp probe failed", zap.String("ip", ip), zap.Error(probeErr), zap.Int("attempt", i+1))
		}
		if alive {
			result.Conflict = true
			result.Method = "arp"
			result.CheckedAt = time.Now().UTC()
			return result, nil
		}
	}
	for i := 0; i < cfg.ICMPRetries; i++ {
		probeCtx, cancel := context.WithTimeout(ctx, s.conflictProbeTimeout())
		alive, _, probeErr := s.conflictProber.ProbeICMP(probeCtx, addr)
		cancel()
		if probeErr != nil && s.logger != nil {
			s.logger.Debug("acd icmp probe failed", zap.String("ip", ip), zap.Error(probeErr), zap.Int("attempt", i+1))
		}
		if alive {
			result.Conflict = true
			result.Method = "icmp"
			result.CheckedAt = time.Now().UTC()
			return result, nil
		}
	}
	result.Method = "none"
	result.CheckedAt = time.Now().UTC()
	return result, nil
}

func (s *Service) acdCheckCandidate(ctx context.Context, poolID, ip string) (bool, error) {
	cfg := s.acdEffectiveConfig(poolID)
	if !cfg.Enabled || s.conflictProber == nil {
		return true, nil
	}
	if cached, ok := s.acdProbeFromCache(ctx, poolID, ip); ok {
		return !cached.Conflict, nil
	}
	result, err := s.acdProbeAddress(ctx, poolID, ip)
	if err != nil {
		return true, nil
	}
	if !result.Conflict {
		s.acdStoreResultAsync(poolID, ip, result, cfg.CacheTTL)
		return true, nil
	}
	if err := s.acdStoreConflictSync(ctx, poolID, ip, result, cfg.ConflictTTL); err != nil && s.logger != nil {
		s.logger.Warn("acd conflict cache write failed", zap.String("pool", poolID), zap.String("ip", ip), zap.Error(err))
	}
	if s.metrics != nil && s.metrics.DHCPv4ACDConflicts != nil {
		s.metrics.DHCPv4ACDConflicts.WithLabelValues(strings.TrimSpace(poolID)).Inc()
	}
	if s.logger != nil {
		payload, _ := json.Marshal(result)
		s.logger.Error("dhcpv4 acd conflict detected",
			zap.String("pool", poolID),
			zap.String("ip", ip),
			zap.String("method", result.Method),
			zap.ByteString("details", payload),
		)
	}
	return false, nil
}
