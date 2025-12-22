package lease

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// ReclaimerOptions controls the auto-reclaim worker behavior.
type ReclaimerOptions struct {
	Interval    time.Duration
	GracePeriod time.Duration
	BatchSize   int
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
	if len(expired) == 0 {
		return nil
	}
	now := time.Now().UTC()
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
	return nil
}
