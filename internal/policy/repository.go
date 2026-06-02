package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"modern-dhcp/internal/cache"
	"modern-dhcp/internal/metrics"
	"modern-dhcp/pkg/models"
)

// Repository abstracts policy rule persistence.
type Repository interface {
	ListRules(ctx context.Context, tenantID string, limit, offset int) ([]models.PolicyRule, error)
	GetRule(ctx context.Context, tenantID, ruleID string) (*models.PolicyRule, error)
	InsertRule(ctx context.Context, rule *models.PolicyRule) error
	UpdateRule(ctx context.Context, rule *models.PolicyRule) error
	DeleteRule(ctx context.Context, tenantID, ruleID string) error
	ReplaceRules(ctx context.Context, tenantID string, rules []models.PolicyRule) error
	ListDrafts(ctx context.Context, tenantID string, limit int) ([]models.PolicyDraft, error)
	GetDraft(ctx context.Context, tenantID, draftID string) (*models.PolicyDraft, error)
	InsertDraft(ctx context.Context, draft *models.PolicyDraft) error
	UpdateDraft(ctx context.Context, draft *models.PolicyDraft) error
	DeleteDraft(ctx context.Context, tenantID, draftID string) error
	InsertVersion(ctx context.Context, version *models.PolicyVersion) error
	ListVersions(ctx context.Context, tenantID string, limit int) ([]models.PolicyVersion, error)
	GetVersion(ctx context.Context, tenantID string, versionNumber int) (*models.PolicyVersion, error)
	LatestVersionNumber(ctx context.Context, tenantID string) (int, error)
}

// MySQLRepository implements Repository via sqlx.
type MySQLRepository struct {
	db       *sqlx.DB
	cache    cache.Store
	cacheTTL time.Duration
	metrics  *metrics.Collector
}

// NewRepository creates a MySQL-backed repository.
func NewRepository(db *sqlx.DB, opts ...RepositoryOption) *MySQLRepository {
	repo := &MySQLRepository{db: db, cacheTTL: 30 * time.Second}
	for _, opt := range opts {
		if opt != nil {
			opt(repo)
		}
	}
	return repo
}

// RepositoryOption customizes repository behavior.
type RepositoryOption func(*MySQLRepository)

// WithCache enables L1+L2 caching for policy rules.
func WithCache(store cache.Store, ttl time.Duration) RepositoryOption {
	return func(r *MySQLRepository) {
		r.cache = store
		if ttl > 0 {
			r.cacheTTL = ttl
		}
	}
}

// WithMetrics enables cache instrumentation.
func WithMetrics(c *metrics.Collector) RepositoryOption {
	return func(r *MySQLRepository) {
		r.metrics = c
	}
}

// ErrRuleNotFound is returned when a policy is missing.
var ErrRuleNotFound = sql.ErrNoRows

func (r *MySQLRepository) cacheKeyRules(tenantID string, limit, offset int) string {
	return fmt.Sprintf("policy:%s:rules:%d:%d", tenantID, limit, offset)
}

func (r *MySQLRepository) cacheKeyRule(tenantID, ruleID string) string {
	return fmt.Sprintf("policy:%s:rule:%s", tenantID, ruleID)
}

func (r *MySQLRepository) recordCache(resource, operation, layer, result string) {
	if r.metrics != nil && r.metrics.CacheEvents != nil {
		r.metrics.CacheEvents.WithLabelValues(resource, operation, layer, result).Inc()
	}
}

func (r *MySQLRepository) evictTenant(ctx context.Context, tenantID string) {
	if r.cache == nil {
		return
	}
	_ = r.cache.DeletePrefix(ctx, fmt.Sprintf("policy:%s:", tenantID))
}

func (r *MySQLRepository) ListRules(ctx context.Context, tenantID string, limit, offset int) ([]models.PolicyRule, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if r.cache != nil {
		var cached []models.PolicyRule
		cacheKey := r.cacheKeyRules(tenantID, limit, offset)
		if ok, cErr := r.cache.GetJSON(ctx, cacheKey, &cached); cErr == nil && ok {
			r.recordCache("policy", "list", "any", "hit")
			return cached, nil
		} else if cErr == nil {
			r.recordCache("policy", "list", "any", "miss")
		}
	}
	const query = `SELECT * FROM policy_rules ORDER BY priority ASC LIMIT ? OFFSET ?`
	var rules []models.PolicyRule
	if err := r.db.SelectContext(ctx, &rules, query, limit, offset); err != nil {
		return nil, err
	}
	if r.cache != nil {
		_ = r.cache.SetJSON(ctx, r.cacheKeyRules(tenantID, limit, offset), rules, r.cacheTTL)
	}
	return rules, nil
}

func (r *MySQLRepository) GetRule(ctx context.Context, tenantID, ruleID string) (*models.PolicyRule, error) {
	const query = `SELECT * FROM policy_rules WHERE id = ?`
	if r.cache != nil {
		var cached models.PolicyRule
		cacheKey := r.cacheKeyRule(tenantID, ruleID)
		if ok, cErr := r.cache.GetJSON(ctx, cacheKey, &cached); cErr == nil && ok {
			r.recordCache("policy", "get", "any", "hit")
			return &cached, nil
		} else if cErr == nil {
			r.recordCache("policy", "get", "any", "miss")
		}
	}
	var rule models.PolicyRule
	if err := r.db.GetContext(ctx, &rule, query, ruleID); err != nil {
		return nil, err
	}
	if r.cache != nil {
		_ = r.cache.SetJSON(ctx, r.cacheKeyRule(tenantID, ruleID), rule, r.cacheTTL)
	}
	return &rule, nil
}

func (r *MySQLRepository) InsertRule(ctx context.Context, rule *models.PolicyRule) error {
	_, err := r.db.NamedExecContext(ctx, `
INSERT INTO policy_rules (
	id, priority, conditions, actions, enabled, created_at, updated_at)
VALUES (
	:id, :priority, :conditions, :actions, :enabled, :created_at, :updated_at)`, rule)
	r.evictTenant(ctx, rule.TenantID)
	return err
}

func (r *MySQLRepository) UpdateRule(ctx context.Context, rule *models.PolicyRule) error {
	rule.UpdatedAt = time.Now().UTC()
	_, err := r.db.NamedExecContext(ctx, `
UPDATE policy_rules SET
    priority = :priority,
    conditions = :conditions,
    actions = :actions,
    enabled = :enabled,
    updated_at = :updated_at
WHERE id = :id`, rule)
	r.evictTenant(ctx, rule.TenantID)
	return err
}

func (r *MySQLRepository) DeleteRule(ctx context.Context, tenantID, ruleID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM policy_rules WHERE id = ?`, ruleID)
	r.evictTenant(ctx, tenantID)
	return err
}

func (r *MySQLRepository) ReplaceRules(ctx context.Context, tenantID string, rules []models.PolicyRule) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `DELETE FROM policy_rules`); err != nil {
		return err
	}
	const insert = `INSERT INTO policy_rules (id, priority, conditions, actions, enabled, created_at, updated_at) VALUES (:id, :priority, :conditions, :actions, :enabled, :created_at, :updated_at)`
	for i := range rules {
		if _, err := tx.NamedExecContext(ctx, insert, &rules[i]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.evictTenant(ctx, tenantID)
	committed = true
	return nil
}

func (r *MySQLRepository) ListDrafts(ctx context.Context, tenantID string, limit int) ([]models.PolicyDraft, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const query = `SELECT id, name, description, status, rules, metadata, created_by, updated_by, created_at, updated_at, published_at FROM policy_drafts ORDER BY updated_at DESC LIMIT ?`
	var drafts []models.PolicyDraft
	if err := r.db.SelectContext(ctx, &drafts, query, limit); err != nil {
		return nil, err
	}
	return drafts, nil
}

func (r *MySQLRepository) GetDraft(ctx context.Context, tenantID, draftID string) (*models.PolicyDraft, error) {
	const query = `SELECT id, name, description, status, rules, metadata, created_by, updated_by, created_at, updated_at, published_at FROM policy_drafts WHERE id = ?`
	var draft models.PolicyDraft
	if err := r.db.GetContext(ctx, &draft, query, draftID); err != nil {
		return nil, err
	}
	return &draft, nil
}

func (r *MySQLRepository) InsertDraft(ctx context.Context, draft *models.PolicyDraft) error {
	const statement = `INSERT INTO policy_drafts (id, name, description, status, rules, metadata, created_by, updated_by, created_at, updated_at, published_at) VALUES (:id, :name, :description, :status, :rules, :metadata, :created_by, :updated_by, :created_at, :updated_at, :published_at)`
	_, err := r.db.NamedExecContext(ctx, statement, draft)
	r.evictTenant(ctx, draft.TenantID)
	return err
}

func (r *MySQLRepository) UpdateDraft(ctx context.Context, draft *models.PolicyDraft) error {
	const statement = `UPDATE policy_drafts SET name = :name, description = :description, status = :status, rules = :rules, metadata = :metadata, updated_by = :updated_by, updated_at = :updated_at, published_at = :published_at WHERE id = :id`
	_, err := r.db.NamedExecContext(ctx, statement, draft)
	r.evictTenant(ctx, draft.TenantID)
	return err
}

func (r *MySQLRepository) DeleteDraft(ctx context.Context, tenantID, draftID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM policy_drafts WHERE id = ?`, draftID)
	r.evictTenant(ctx, tenantID)
	return err
}

func (r *MySQLRepository) InsertVersion(ctx context.Context, version *models.PolicyVersion) error {
	const statement = `INSERT INTO policy_versions (id, version, derived_from, changelog, rules, metadata, published_by, published_at, rollback_of, created_at) VALUES (:id, :version, :derived_from, :changelog, :rules, :metadata, :published_by, :published_at, :rollback_of, :created_at)`
	_, err := r.db.NamedExecContext(ctx, statement, version)
	r.evictTenant(ctx, version.TenantID)
	return err
}

func (r *MySQLRepository) ListVersions(ctx context.Context, tenantID string, limit int) ([]models.PolicyVersion, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const query = `SELECT id, version, derived_from, changelog, rules, metadata, published_by, published_at, rollback_of, created_at FROM policy_versions ORDER BY version DESC LIMIT ?`
	var versions []models.PolicyVersion
	if err := r.db.SelectContext(ctx, &versions, query, limit); err != nil {
		return nil, err
	}
	return versions, nil
}

func (r *MySQLRepository) GetVersion(ctx context.Context, tenantID string, versionNumber int) (*models.PolicyVersion, error) {
	const query = `SELECT id, version, derived_from, changelog, rules, metadata, published_by, published_at, rollback_of, created_at FROM policy_versions WHERE version = ?`
	var version models.PolicyVersion
	if err := r.db.GetContext(ctx, &version, query, versionNumber); err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *MySQLRepository) LatestVersionNumber(ctx context.Context, tenantID string) (int, error) {
	const query = `SELECT version FROM policy_versions ORDER BY version DESC LIMIT 1`
	var number sql.NullInt64
	if err := r.db.GetContext(ctx, &number, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	if !number.Valid {
		return 0, nil
	}
	return int(number.Int64), nil
}
