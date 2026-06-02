package lease

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// ReclaimerOptions controls the auto-reclaim worker behavior.
type ReclaimerOptions struct {
	Interval                     time.Duration
	GracePeriod                  time.Duration
	BatchSize                    int
	HistoryRetentionDays         int
	HistoryPurgeBatchSize        int
	LegacyActiveCleanupBatchSize int
}

// Reclaimer periodically releases expired leases after a grace period.
type Reclaimer struct {
	repo    Repository
	logger  *zap.Logger
	options ReclaimerOptions
}

// NewReclaimer constructs a new reclaim worker.
func NewReclaimer(repo Repository, logger *zap.Logger, opts ReclaimerOptions) *Reclaimer {
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.GracePeriod <= 0 {
		opts.GracePeriod = 5 * time.Minute
	}
	if opts.BatchSize <= 0 || opts.BatchSize > 500 {
		opts.BatchSize = 100
	}
	if opts.HistoryRetentionDays <= 0 {
		opts.HistoryRetentionDays = 90
	}
	if opts.HistoryPurgeBatchSize <= 0 || opts.HistoryPurgeBatchSize > 5000 {
		opts.HistoryPurgeBatchSize = 1000
	}
	if opts.LegacyActiveCleanupBatchSize <= 0 || opts.LegacyActiveCleanupBatchSize > 5000 {
		opts.LegacyActiveCleanupBatchSize = 1000
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Reclaimer{repo: repo, logger: logger, options: opts}
}

// Run starts the reclaim loop until the context is canceled.
func (r *Reclaimer) Run(ctx context.Context) {
	ticker := time.NewTicker(r.options.Interval)
	defer ticker.Stop()
	r.logger.Info("lease reclaimer started", zap.Duration("interval", r.options.Interval))
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("lease reclaimer stopped")
			return
		case <-ticker.C:
			if err := r.reclaimBatch(ctx); err != nil {
				r.logger.Warn("lease reclaim batch failed", zap.Error(err))
			}
		}
	}
}

func (r *Reclaimer) reclaimBatch(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-r.options.GracePeriod)
	expired, err := r.repo.ListExpiredLeases(ctx, cutoff, r.options.BatchSize)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if len(expired) > 0 {
		if err := r.repo.ArchiveLeases(ctx, expired, now); err != nil {
			r.logger.Warn("archive expired leases failed", zap.Error(err))
		}
	}
	var reclaimed int
	for i := range expired {
		lease := expired[i]
		lease.State = leaseStateReleased
		lease.UpdatedAt = now
		lease.ExpiresAt = now
		lease.CooldownUntil = nil
		if err := r.repo.UpdateLease(ctx, &lease); err != nil {
			r.logger.Warn("failed to reclaim lease", zap.String("leaseId", lease.ID), zap.Error(err))
			continue
		}
		reclaimed++
	}
	if reclaimed > 0 {
		r.logger.Info("reclaimed expired leases", zap.Int("count", reclaimed))
	}
	cleaned, cleanErr := r.repo.CleanupLegacyActiveSnapshots(ctx, r.options.LegacyActiveCleanupBatchSize)
	if cleanErr != nil {
		r.logger.Warn("cleanup legacy active snapshots failed", zap.Error(cleanErr))
	} else if cleaned > 0 {
		r.logger.Info("cleaned legacy active snapshots", zap.Int64("count", cleaned))
	}
	if r.options.HistoryRetentionDays > 0 {
		purgeBefore := now.AddDate(0, 0, -r.options.HistoryRetentionDays)
		purged, purgeErr := r.repo.PurgeLeaseHistory(ctx, purgeBefore, r.options.HistoryPurgeBatchSize)
		if purgeErr != nil {
			r.logger.Warn("purge lease history failed", zap.Error(purgeErr))
		} else if purged > 0 {
			r.logger.Info("purged archived lease history", zap.Int64("count", purged), zap.Time("before", purgeBefore))
		}
	}
	return nil
}
