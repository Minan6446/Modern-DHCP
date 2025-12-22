package backup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// Manager coordinates backup scheduling metadata and exposes health hooks.
type Manager struct {
	cfg             config.BackupConfig
	logger          *zap.Logger
	mu              sync.RWMutex
	lastFull        time.Time
	lastIncremental time.Time
}

// NewManager instantiates a backup manager if backups are enabled.
func NewManager(cfg config.BackupConfig, logger *zap.Logger) *Manager {
	if !cfg.Enabled {
		return nil
	}
	return &Manager{cfg: cfg, logger: logger}
}

// Name implements the server.HealthHook contract.
func (m *Manager) Name() string {
	return "backup"
}

// Check reports whether scheduled backups have recently executed.
func (m *Manager) Check(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg.FullInterval > 0 && !m.lastFull.IsZero() {
		if time.Since(m.lastFull) > m.cfg.FullInterval*2 {
			return fmt.Errorf("last full backup stale: %s", time.Since(m.lastFull).Round(time.Minute))
		}
	}
	if m.cfg.IncrementalInterval > 0 && !m.lastIncremental.IsZero() {
		if time.Since(m.lastIncremental) > m.cfg.IncrementalInterval*2 {
			return fmt.Errorf("last incremental backup stale: %s", time.Since(m.lastIncremental).Round(time.Minute))
		}
	}
	return nil
}

// Start begins emitting backup events based on configured intervals.
func (m *Manager) Start(ctx context.Context) {
	m.logger.Info("backup scheduler started",
		zap.Bool("crossRegion", m.cfg.CrossRegion.Enabled),
		zap.Strings("locations", m.cfg.Locations),
		zap.String("schedule", m.cfg.Schedule),
		zap.Duration("fullInterval", m.cfg.FullInterval),
		zap.Duration("incrementalInterval", m.cfg.IncrementalInterval),
	)
	if m.cfg.FullInterval > 0 {
		go m.runTicker(ctx, m.cfg.FullInterval, "full")
	}
	if m.cfg.IncrementalInterval > 0 {
		go m.runTicker(ctx, m.cfg.IncrementalInterval, "incremental")
	}
}

func (m *Manager) runTicker(ctx context.Context, interval time.Duration, kind string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	m.logger.Info("backup ticker armed", zap.String("type", kind), zap.Duration("interval", interval))
	for {
		select {
		case <-ctx.Done():
			m.logger.Info("backup ticker stopped", zap.String("type", kind))
			return
		case now := <-ticker.C:
			m.recordRun(now, kind)
		}
	}
}

func (m *Manager) recordRun(ts time.Time, kind string) {
	switch kind {
	case "full":
		m.mu.Lock()
		m.lastFull = ts
		m.mu.Unlock()
	case "incremental":
		m.mu.Lock()
		m.lastIncremental = ts
		m.mu.Unlock()
	}
	m.logger.Info("backup execution signaled",
		zap.String("type", kind),
		zap.Time("scheduledAt", ts),
		zap.Strings("targets", m.cfg.Locations),
		zap.Bool("crossRegion", m.cfg.CrossRegion.Enabled),
	)
}
