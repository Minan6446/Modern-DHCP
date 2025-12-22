package tenant

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"modern-dhcp/pkg/models"
)

var (
	// ErrPoolQuotaExceeded indicates the tenant reached the configured pool cap.
	ErrPoolQuotaExceeded = errors.New("tenant quota exceeded: address pools")
	// ErrLeaseQuotaExceeded indicates the tenant reached the configured lease cap.
	ErrLeaseQuotaExceeded = errors.New("tenant quota exceeded: leases")
)

// PoolCounter exposes the minimal method required to count pools.
type PoolCounter interface {
	CountPools(ctx context.Context, tenantID string) (int, error)
}

// LeaseCounter exposes the minimal method required to count active leases.
type LeaseCounter interface {
	CountActiveLeases(ctx context.Context, tenantID string) (int, error)
}

// QuotaRepository persists per-tenant quota metadata.
type QuotaRepository interface {
	GetQuota(ctx context.Context, tenantID string) (models.TenantQuota, error)
	UpsertQuota(ctx context.Context, quota models.TenantQuota) error
}

// QuotaEnforcer exposes the enforcement hooks required by services.
type QuotaEnforcer interface {
	EnsurePoolCapacity(ctx context.Context, tenantID string) error
	EnsureLeaseCapacity(ctx context.Context, tenantID string) error
}

// Service coordinates quota checks across repositories.
type Service struct {
	repo   QuotaRepository
	pools  PoolCounter
	leases LeaseCounter
	logger *zap.Logger
}

// NewQuotaService constructs a quota service.
func NewQuotaService(repo QuotaRepository, pools PoolCounter, leases LeaseCounter, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{repo: repo, pools: pools, leases: leases, logger: logger}
}

// GetQuota returns the current quota metadata for a tenant.
func (s *Service) GetQuota(ctx context.Context, tenantID string) (models.TenantQuota, error) {
	if s == nil || s.repo == nil {
		return models.TenantQuota{}, errors.New("tenant quota repository unavailable")
	}
	return s.repo.GetQuota(ctx, tenantID)
}

// SaveQuota persists new quota limits and returns the updated row.
func (s *Service) SaveQuota(ctx context.Context, quota models.TenantQuota) (models.TenantQuota, error) {
	if s == nil || s.repo == nil {
		return models.TenantQuota{}, errors.New("tenant quota repository unavailable")
	}
	tenantID := strings.TrimSpace(quota.TenantID)
	if tenantID == "" {
		return models.TenantQuota{}, errors.New("tenant id is required")
	}
	quota.TenantID = tenantID
	if err := s.repo.UpsertQuota(ctx, quota); err != nil {
		return models.TenantQuota{}, err
	}
	return s.repo.GetQuota(ctx, tenantID)
}

// EnsurePoolCapacity validates whether another pool can be created.
func (s *Service) EnsurePoolCapacity(ctx context.Context, tenantID string) error {
	quota, err := s.repo.GetQuota(ctx, tenantID)
	if err != nil {
		return err
	}
	if quota.PoolLimit <= 0 {
		return nil
	}
	count, err := s.pools.CountPools(ctx, tenantID)
	if err != nil {
		return err
	}
	if count >= quota.PoolLimit {
		s.logger.Warn("tenant pool quota exceeded", zap.String("tenantId", tenantID), zap.Int("limit", quota.PoolLimit))
		return ErrPoolQuotaExceeded
	}
	return nil
}

// EnsureLeaseCapacity validates whether another active lease can be issued.
func (s *Service) EnsureLeaseCapacity(ctx context.Context, tenantID string) error {
	quota, err := s.repo.GetQuota(ctx, tenantID)
	if err != nil {
		return err
	}
	if quota.LeaseLimit <= 0 {
		return nil
	}
	count, err := s.leases.CountActiveLeases(ctx, tenantID)
	if err != nil {
		return err
	}
	if count >= quota.LeaseLimit {
		s.logger.Warn("tenant lease quota exceeded", zap.String("tenantId", tenantID), zap.Int("limit", quota.LeaseLimit))
		return ErrLeaseQuotaExceeded
	}
	return nil
}

// SQLQuotaRepository stores quota data in tenant_quotas.
type SQLQuotaRepository struct {
	db *sqlx.DB
}

// NewQuotaRepository builds a quota repo backed by sqlx.
func NewQuotaRepository(db *sqlx.DB) *SQLQuotaRepository {
	return &SQLQuotaRepository{db: db}
}

// GetQuota fetches quota row or returns zeroed defaults.
func (r *SQLQuotaRepository) GetQuota(ctx context.Context, tenantID string) (models.TenantQuota, error) {
	const query = `SELECT tenant_id, pool_limit, lease_limit, client_limit, api_request_limit, automation_job_limit, updated_at FROM tenant_quotas WHERE tenant_id = ?`
	var quota models.TenantQuota
	if err := r.db.GetContext(ctx, &quota, query, tenantID); err != nil {
		if err == sql.ErrNoRows {
			return models.TenantQuota{TenantID: tenantID}, nil
		}
		return models.TenantQuota{}, err
	}
	return quota, nil
}

// UpsertQuota persists limits for a tenant.
func (r *SQLQuotaRepository) UpsertQuota(ctx context.Context, quota models.TenantQuota) error {
	const stmt = `
	INSERT INTO tenant_quotas (tenant_id, pool_limit, lease_limit, client_limit, api_request_limit, automation_job_limit, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		pool_limit = VALUES(pool_limit),
		lease_limit = VALUES(lease_limit),
		client_limit = VALUES(client_limit),
		api_request_limit = VALUES(api_request_limit),
		automation_job_limit = VALUES(automation_job_limit),
		updated_at = VALUES(updated_at)`
	_, err := r.db.ExecContext(ctx, stmt, quota.TenantID, quota.PoolLimit, quota.LeaseLimit, quota.ClientLimit, quota.APIRequestLimit, quota.AutomationJobLimit, time.Now().UTC())
	return err
}

var _ QuotaRepository = (*SQLQuotaRepository)(nil)
var _ QuotaEnforcer = (*Service)(nil)
