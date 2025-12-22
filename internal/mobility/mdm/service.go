package mdm

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// Service coordinates polling across connectors and exposes a cache lookup API for DHCP flows.
type Service struct {
	connectors   []Connector
	cache        *Cache
	pollInterval time.Duration
	logger       *zap.Logger
	once         sync.Once
}

// NewService builds the MDM service. Returns nil when disabled or no connectors are active.
func NewService(cfg config.MDMConfig, logger *zap.Logger) *Service {
	if !cfg.Enabled {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	connectors := buildConnectors(cfg, logger)
	if len(connectors) == 0 {
		logger.Info("mdm service disabled", zap.String("reason", "no connectors enabled"))
		return nil
	}
	interval := cfg.PollInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	cache := NewCache(cfg.CacheTTL)
	return &Service{
		connectors:   connectors,
		cache:        cache,
		pollInterval: interval,
		logger:       logger,
	}
}

// Run starts the periodic sync loop. It should be invoked in a goroutine.
func (s *Service) Run(ctx context.Context) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.logger.Info("mdm service started", zap.Int("connectors", len(s.connectors)), zap.Duration("interval", s.pollInterval))
	})
	s.sync(ctx)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("mdm service stopping", zap.Error(ctx.Err()))
			return
		case <-ticker.C:
			s.sync(ctx)
		}
	}
}

// Lookup returns the cached compliance record for the provided tenant/device identifiers.
func (s *Service) Lookup(tenantID, deviceID string) (ComplianceRecord, bool) {
	if s == nil || s.cache == nil {
		return ComplianceRecord{}, false
	}
	return s.cache.Lookup(tenantID, deviceID)
}

func (s *Service) sync(ctx context.Context) {
	if s == nil {
		return
	}
	for _, connector := range s.connectors {
		if !connector.Enabled() {
			continue
		}
		records, err := connector.Sync(ctx)
		if err != nil {
			s.logger.Warn("mdm sync failed", zap.String("connector", connector.Name()), zap.Error(err))
			continue
		}
		for _, record := range records {
			s.cache.Remember(record)
		}
	}
}
