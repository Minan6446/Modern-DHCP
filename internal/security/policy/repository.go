package policy

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "modern-dhcp/internal/db"
)

// Repository persists tenant security policies.
type Repository interface {
	ListRules(ctx context.Context, tenantID string) ([]Rule, error)
	GetRule(ctx context.Context, tenantID, ruleID string) (*Rule, error)
	CreateRule(ctx context.Context, rule *Rule) error
	UpdateRule(ctx context.Context, rule *Rule) error
	DeleteRule(ctx context.Context, tenantID, ruleID string) error
}

var (
	// ErrRuleNotFound signals an unknown rule identifier.
	ErrRuleNotFound = errors.New("security policy: rule not found")
)

type mysqlRepository struct {
	db *sqlx.DB
}

// NewRepository returns a Repository backed by MySQL/Postgres via sqlx.
func NewRepository(db *sqlx.DB) Repository {
	return &mysqlRepository{db: db}
}

func (r *mysqlRepository) ListRules(ctx context.Context, tenantID string) ([]Rule, error) {
	const query = `
SELECT id, name, description, priority, effect, enabled, created_at, updated_at
FROM security_policy_rules
WHERE 1=1
ORDER BY priority ASC, created_at ASC`
	rows := make([]ruleRecord, 0)
	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	ruleIDs := make([]string, 0, len(rows))
	for _, rec := range rows {
		ruleIDs = append(ruleIDs, rec.ID)
	}
	matchMap, err := r.loadMatches(ctx, ruleIDs)
	if err != nil {
		return nil, err
	}
	result := make([]Rule, 0, len(rows))
	for _, rec := range rows {
		rule := rec.ToModel()
		rule.Matches = append(rule.Matches, matchMap[rule.ID]...)
		result = append(result, rule)
	}
	return result, nil
}

func (r *mysqlRepository) GetRule(ctx context.Context, tenantID, ruleID string) (*Rule, error) {
	const query = `
SELECT id, name, description, priority, effect, enabled, created_at, updated_at
FROM security_policy_rules
WHERE id = ?
LIMIT 1`
	var rec ruleRecord
	if err := r.db.GetContext(ctx, &rec, query, ruleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRuleNotFound
		}
		return nil, err
	}
	matches, err := r.loadMatches(ctx, []string{rec.ID})
	if err != nil {
		return nil, err
	}
	rule := rec.ToModel()
	rule.Matches = append(rule.Matches, matches[rule.ID]...)
	return &rule, nil
}

func (r *mysqlRepository) CreateRule(ctx context.Context, rule *Rule) error {
	if rule == nil {
		return errors.New("security policy: rule required")
	}
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	if rule.ID == "" {
		return errors.New("security policy: id required")
	}
	const insertRule = `
INSERT INTO security_policy_rules (id, name, description, priority, effect, enabled, created_at, updated_at)
VALUES (:id, :name, :description, :priority, :effect, :enabled, :created_at, :updated_at)`
	return dbutil.WithTx(ctx, r.db, func(tx *sqlx.Tx) error {
		params := map[string]any{
			"id":          rule.ID,
			"name":        rule.Name,
			"description": rule.Description,
			"priority":    rule.Priority,
			"effect":      strings.ToLower(string(rule.Effect)),
			"enabled":     boolToInt(rule.Enabled),
			"created_at":  rule.CreatedAt,
			"updated_at":  rule.UpdatedAt,
		}
		if _, err := tx.NamedExecContext(ctx, insertRule, params); err != nil {
			return err
		}
		return r.persistMatches(ctx, tx, rule)
	})
}

func (r *mysqlRepository) UpdateRule(ctx context.Context, rule *Rule) error {
	if rule == nil {
		return errors.New("security policy: rule required")
	}
	now := time.Now().UTC()
	rule.UpdatedAt = now
	const updateRule = `
UPDATE security_policy_rules
SET name = :name,
    description = :description,
    priority = :priority,
    effect = :effect,
    enabled = :enabled,
	updated_at = :updated_at
WHERE id = :id`
	return dbutil.WithTx(ctx, r.db, func(tx *sqlx.Tx) error {
		params := map[string]any{
			"id":          rule.ID,
			"name":        rule.Name,
			"description": rule.Description,
			"priority":    rule.Priority,
			"effect":      strings.ToLower(string(rule.Effect)),
			"enabled":     boolToInt(rule.Enabled),
			"updated_at":  rule.UpdatedAt,
		}
		res, err := tx.NamedExecContext(ctx, updateRule, params)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return ErrRuleNotFound
		}
		if err := r.deleteMatches(ctx, tx, rule.ID); err != nil {
			return err
		}
		return r.persistMatches(ctx, tx, rule)
	})
}

func (r *mysqlRepository) DeleteRule(ctx context.Context, tenantID, ruleID string) error {
	const deleteRule = `DELETE FROM security_policy_rules WHERE id = ?`
	return dbutil.WithTx(ctx, r.db, func(tx *sqlx.Tx) error {
		if err := r.deleteMatches(ctx, tx, ruleID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, deleteRule, ruleID)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return ErrRuleNotFound
		}
		return nil
	})
}

func (r *mysqlRepository) loadMatches(ctx context.Context, ruleIDs []string) (map[string][]Match, error) {
	result := make(map[string][]Match, len(ruleIDs))
	if len(ruleIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
SELECT id, rule_id, match_type, match_value, negate, created_at
FROM security_policy_matches
WHERE rule_id IN (?)
ORDER BY id ASC`, ruleIDs)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)
	rows := make([]matchRecord, 0)
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, rec := range rows {
		result[rec.RuleID] = append(result[rec.RuleID], rec.ToModel())
	}
	return result, nil
}

func (r *mysqlRepository) persistMatches(ctx context.Context, tx *sqlx.Tx, rule *Rule) error {
	if rule == nil || len(rule.Matches) == 0 {
		return nil
	}
	const insertMatch = `
INSERT INTO security_policy_matches (rule_id, match_type, match_value, negate, created_at)
VALUES (:rule_id, :match_type, :match_value, :negate, :created_at)`
	for _, match := range rule.Matches {
		params := map[string]any{
			"rule_id":     rule.ID,
			"match_type":  strings.ToLower(string(match.Type)),
			"match_value": strings.TrimSpace(match.Value),
			"negate":      boolToInt(match.Negate),
			"created_at":  rule.UpdatedAt,
		}
		if _, err := tx.NamedExecContext(ctx, insertMatch, params); err != nil {
			return err
		}
	}
	return nil
}

func (r *mysqlRepository) deleteMatches(ctx context.Context, tx *sqlx.Tx, ruleID string) error {
	const deleteMatches = `DELETE FROM security_policy_matches WHERE rule_id = ?`
	if _, err := tx.ExecContext(ctx, deleteMatches, ruleID); err != nil {
		return err
	}
	return nil
}

type ruleRecord struct {
	ID          string    `db:"id"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	Priority    int       `db:"priority"`
	Effect      string    `db:"effect"`
	Enabled     bool      `db:"enabled"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func (r ruleRecord) ToModel() Rule {
	return Rule{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Priority:    r.Priority,
		Effect:      ParseEffect(r.Effect),
		Enabled:     r.Enabled,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

type matchRecord struct {
	ID        uint64    `db:"id"`
	RuleID    string    `db:"rule_id"`
	Type      string    `db:"match_type"`
	Value     string    `db:"match_value"`
	Negate    bool      `db:"negate"`
	CreatedAt time.Time `db:"created_at"`
}

func (m matchRecord) ToModel() Match {
	return Match{
		ID:        m.ID,
		RuleID:    m.RuleID,
		Type:      MatchType(strings.ToLower(m.Type)),
		Value:     m.Value,
		Negate:    m.Negate,
		CreatedAt: m.CreatedAt,
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
