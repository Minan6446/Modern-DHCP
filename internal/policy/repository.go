package policy

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"modern-dhcp/pkg/models"
)

// Repository abstracts policy rule persistence.
type Repository interface {
	ListRules(ctx context.Context, tenantID string, limit, offset int) ([]models.PolicyRule, error)
	GetRule(ctx context.Context, tenantID, ruleID string) (*models.PolicyRule, error)
	InsertRule(ctx context.Context, rule *models.PolicyRule) error
	UpdateRule(ctx context.Context, rule *models.PolicyRule) error
	DeleteRule(ctx context.Context, tenantID, ruleID string) error
}

// MySQLRepository implements Repository via sqlx.
type MySQLRepository struct {
	db *sqlx.DB
}

// NewRepository creates a MySQL-backed repository.
func NewRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// ErrRuleNotFound is returned when a policy is missing.
var ErrRuleNotFound = sql.ErrNoRows

func (r *MySQLRepository) ListRules(ctx context.Context, tenantID string, limit, offset int) ([]models.PolicyRule, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	const query = `SELECT * FROM policy_rules WHERE tenant_id = ? ORDER BY priority ASC LIMIT ? OFFSET ?`
	var rules []models.PolicyRule
	if err := r.db.SelectContext(ctx, &rules, query, tenantID, limit, offset); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *MySQLRepository) GetRule(ctx context.Context, tenantID, ruleID string) (*models.PolicyRule, error) {
	const query = `SELECT * FROM policy_rules WHERE tenant_id = ? AND id = ?`
	var rule models.PolicyRule
	if err := r.db.GetContext(ctx, &rule, query, tenantID, ruleID); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *MySQLRepository) InsertRule(ctx context.Context, rule *models.PolicyRule) error {
	_, err := r.db.NamedExecContext(ctx, `
INSERT INTO policy_rules (
    id, tenant_id, priority, conditions, actions, enabled, created_at, updated_at)
VALUES (
    :id, :tenant_id, :priority, :conditions, :actions, :enabled, :created_at, :updated_at)`, rule)
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
WHERE id = :id AND tenant_id = :tenant_id`, rule)
	return err
}

func (r *MySQLRepository) DeleteRule(ctx context.Context, tenantID, ruleID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM policy_rules WHERE tenant_id = ? AND id = ?`, tenantID, ruleID)
	return err
}
