package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/automation/workflow"
	"modern-dhcp/pkg/models"
)

const (
	// ActionLeaseReclaim identifies the workflow task for reclaiming expired leases.
	ActionLeaseReclaim = "lease.reclaim"

	defaultReclaimBatch = 100
	maxReclaimBatch     = 500
	defaultGracePeriod  = 5 * time.Minute

	leaseStateReleased = "RELEASED"
)

// LeaseRepository exposes the subset of lease persistence needed for reclaim tasks.
type LeaseRepository interface {
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]models.Lease, error)
	UpdateLease(ctx context.Context, lease *models.Lease) error
}

// NewLeaseReclaimHandler returns a workflow task handler that reclaims expired leases in batches.
func NewLeaseReclaimHandler(repo LeaseRepository, logger *zap.Logger) workflow.TaskHandler {
	if repo == nil {
		panic("workflow/actions: lease repository required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(ctx context.Context, task workflow.TaskContext) (workflow.TaskResult, error) {
		params := parseLeaseReclaimParams(task.Node.Task)
		cutoff := time.Now().UTC().Add(-params.GracePeriod)
		expired, err := repo.ListExpiredLeases(ctx, cutoff, params.BatchSize)
		if err != nil {
			return workflow.TaskResult{}, fmt.Errorf("lease.reclaim: list expired leases: %w", err)
		}
		metadata := map[string]any{
			"expired":            len(expired),
			"batchSize":          params.BatchSize,
			"gracePeriodSeconds": int(params.GracePeriod.Seconds()),
		}
		if len(expired) == 0 {
			return workflow.TaskResult{
				Summary:  "no expired leases",
				Metadata: metadata,
			}, nil
		}
		now := time.Now().UTC()
		var reclaimed, failed int
		for i := range expired {
			if err := ctx.Err(); err != nil {
				return workflow.TaskResult{}, err
			}
			lease := expired[i]
			lease.State = leaseStateReleased
			lease.UpdatedAt = now
			lease.ExpiresAt = now
			lease.CooldownUntil = nil
			if err := repo.UpdateLease(ctx, &lease); err != nil {
				failed++
				logger.Warn("workflow lease reclaim update failed",
					zap.String("leaseId", lease.ID),
					zap.String("tenantId", lease.TenantID),
					zap.Error(err))
				continue
			}
			reclaimed++
		}
		metadata["reclaimed"] = reclaimed
		metadata["failed"] = failed
		summary := fmt.Sprintf("reclaimed %d/%d expired leases", reclaimed, len(expired))
		if failed > 0 {
			summary = fmt.Sprintf("%s (%d failed)", summary, failed)
		}
		return workflow.TaskResult{Summary: summary, Metadata: metadata}, nil
	}
}

type leaseReclaimParams struct {
	GracePeriod time.Duration
	BatchSize   int
}

func parseLeaseReclaimParams(task *workflow.TaskNode) leaseReclaimParams {
	params := leaseReclaimParams{
		GracePeriod: defaultGracePeriod,
		BatchSize:   defaultReclaimBatch,
	}
	if task == nil || len(task.Params) == 0 {
		return params
	}
	if value, ok := task.Params["batchSize"]; ok {
		if size, ok := toInt(value); ok && size > 0 {
			if size > maxReclaimBatch {
				size = maxReclaimBatch
			}
			params.BatchSize = size
		}
	}
	if value, ok := task.Params["gracePeriod"]; ok {
		if dur, ok := toDuration(value); ok && dur > 0 {
			params.GracePeriod = dur
		}
	} else if value, ok := task.Params["graceSeconds"]; ok {
		if dur, ok := toDuration(value); ok && dur > 0 {
			params.GracePeriod = dur
		}
	}
	return params
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func toDuration(value any) (time.Duration, bool) {
	switch v := value.(type) {
	case time.Duration:
		return v, true
	case int:
		return time.Duration(v) * time.Second, true
	case int64:
		return time.Duration(v) * time.Second, true
	case float64:
		return time.Duration(v) * time.Second, true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return time.Duration(f) * time.Second, true
	case string:
		if d, err := time.ParseDuration(v); err == nil {
			return d, true
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return time.Duration(f) * time.Second, true
		}
		return 0, false
	default:
		return 0, false
	}
}
