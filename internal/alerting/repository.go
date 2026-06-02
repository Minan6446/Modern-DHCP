package alerting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	dbutil "modern-dhcp/internal/db"

	"github.com/jmoiron/sqlx"
)

// Rule represents a persisted alert rule definition.
type Rule struct {
	ID            string    `db:"id" json:"id"`
	TenantID      string    `db:"tenant_id" json:"tenantId"`
	Name          string    `db:"name" json:"name"`
	Expression    string    `db:"expression" json:"expression"`
	Operator      string    `db:"operator" json:"operator"`
	Threshold     float64   `db:"threshold" json:"threshold"`
	DurationSec   int       `db:"duration_sec" json:"durationSec"`
	Severity      string    `db:"severity" json:"severity"`
	Enabled       bool      `db:"enabled" json:"enabled"`
	MatchTemplate string    `db:"match_template" json:"matchTemplate"`
	CreatedAt     time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt     time.Time `db:"updated_at" json:"updatedAt"`
}

// RuleStore persists alert rules.
type RuleStore interface {
	ListRules(ctx context.Context, tenantID, keyword string, limit, offset int) ([]Rule, int, error)
	UpsertRule(ctx context.Context, rule *Rule) error
	DeleteRule(ctx context.Context, tenantID, id string) error
}

// RouteStore persists routing rules.
type RouteStore interface {
	ListRoutes(ctx context.Context) ([]RoutingRule, error)
	ReplaceRoutes(ctx context.Context, rules []RoutingRule, actor string) error
}

// SQLStore implements RuleStore and RouteStore on MySQL.
type SQLStore struct {
	db *sqlx.DB
}

// NewSQLStore constructs a SQL-backed alert store.
func NewSQLStore(db *sqlx.DB) *SQLStore {
	if db == nil {
		return nil
	}
	return &SQLStore{db: db}
}

// ListRules returns paged rules for a tenant with optional keyword filtering.
func (s *SQLStore) ListRules(ctx context.Context, tenantID, keyword string, limit, offset int) ([]Rule, int, error) {
	if s == nil {
		return nil, 0, errors.New("alert store unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, 0, errors.New("tenantID required")
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	like := "%%"
	args := []any{tenantID}
	countArgs := []any{tenantID}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like = "%" + keyword + "%"
		args = append(args, like, like)
		countArgs = append(countArgs, like, like)
	}
	query := `SELECT id, tenant_id, name, expression, operator, threshold, duration_sec, severity, enabled, match_template, created_at, updated_at
              FROM alert_rules
              WHERE tenant_id = ?`
	countQuery := `SELECT COUNT(1) FROM alert_rules WHERE tenant_id = ?`
	if keyword != "" {
		query += " AND (name LIKE ? OR expression LIKE ?)"
		countQuery += " AND (name LIKE ? OR expression LIKE ?)"
	}
	query += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows := []Rule{}
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.GetContext(ctx, &total, countQuery, countArgs...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// UpsertRule inserts or updates a rule.
func (s *SQLStore) UpsertRule(ctx context.Context, rule *Rule) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	if rule == nil {
		return errors.New("rule required")
	}
	rule.TenantID = strings.TrimSpace(rule.TenantID)
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Expression = strings.TrimSpace(rule.Expression)
	rule.Operator = strings.TrimSpace(rule.Operator)
	rule.Severity = strings.TrimSpace(rule.Severity)
	if rule.TenantID == "" || rule.ID == "" || rule.Name == "" || rule.Expression == "" || rule.Operator == "" || rule.Severity == "" {
		return errors.New("id, tenantId, name, expression, operator, severity required")
	}
	now := time.Now().UTC()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	const stmt = `INSERT INTO alert_rules
(id, tenant_id, name, expression, operator, threshold, duration_sec, severity, enabled, match_template, created_at, updated_at)
VALUES (:id, :tenant_id, :name, :expression, :operator, :threshold, :duration_sec, :severity, :enabled, :match_template, :created_at, :updated_at)
ON DUPLICATE KEY UPDATE
  name=VALUES(name),
  expression=VALUES(expression),
  operator=VALUES(operator),
  threshold=VALUES(threshold),
  duration_sec=VALUES(duration_sec),
  severity=VALUES(severity),
  enabled=VALUES(enabled),
  match_template=VALUES(match_template),
  updated_at=VALUES(updated_at)`
	payload := map[string]any{
		"id":             rule.ID,
		"tenant_id":      rule.TenantID,
		"name":           rule.Name,
		"expression":     rule.Expression,
		"operator":       strings.ToLower(rule.Operator),
		"threshold":      rule.Threshold,
		"duration_sec":   rule.DurationSec,
		"severity":       strings.ToLower(rule.Severity),
		"enabled":        boolToInt(rule.Enabled),
		"match_template": rule.MatchTemplate,
		"created_at":     rule.CreatedAt,
		"updated_at":     rule.UpdatedAt,
	}
	_, err := s.db.NamedExecContext(ctx, stmt, payload)
	return err
}

// DeleteRule removes a rule by id scoped to tenant.
func (s *SQLStore) DeleteRule(ctx context.Context, tenantID, id string) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	const stmt = `DELETE FROM alert_rules WHERE tenant_id = ? AND id = ?`
	res, err := s.db.ExecContext(ctx, stmt, strings.TrimSpace(tenantID), strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListRoutes returns all routing rules.
func (s *SQLStore) ListRoutes(ctx context.Context) ([]RoutingRule, error) {
	if s == nil {
		return nil, errors.New("alert store unavailable")
	}
	const query = `SELECT id, name, description, severities, channels, escalation_minutes, enabled, updated_at, updated_by FROM alert_routes ORDER BY updated_at DESC`
	type row struct {
		ID                int64          `db:"id"`
		Name              string         `db:"name"`
		Description       sql.NullString `db:"description"`
		SeveritiesRaw     string         `db:"severities"`
		ChannelsRaw       string         `db:"channels"`
		EscalationMinutes int            `db:"escalation_minutes"`
		Enabled           bool           `db:"enabled"`
		UpdatedAt         time.Time      `db:"updated_at"`
		UpdatedBy         string         `db:"updated_by"`
	}
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	result := make([]RoutingRule, 0, len(rows))
	for _, r := range rows {
		severities, channels := []string{}, []string{}
		_ = json.Unmarshal([]byte(r.SeveritiesRaw), &severities)
		_ = json.Unmarshal([]byte(r.ChannelsRaw), &channels)
		result = append(result, RoutingRule{
			Name:              r.Name,
			Description:       strings.TrimSpace(r.Description.String),
			Severities:        severities,
			Channels:          channels,
			EscalationMinutes: r.EscalationMinutes,
			Enabled:           r.Enabled,
			UpdatedAt:         r.UpdatedAt,
			UpdatedBy:         r.UpdatedBy,
		})
	}
	return result, nil
}

// ReplaceRoutes swaps the routing rules transactionally.
func (s *SQLStore) ReplaceRoutes(ctx context.Context, rules []RoutingRule, actor string) error {
	if s == nil {
		return errors.New("alert store unavailable")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}
	return dbutil.WithTx(ctx, s.db, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM alert_routes"); err != nil {
			return err
		}
		if len(rules) == 0 {
			return nil
		}
		const stmt = `INSERT INTO alert_routes (name, description, severities, channels, escalation_minutes, enabled, updated_at, updated_by)
VALUES (:name, :description, :severities, :channels, :escalation_minutes, :enabled, :updated_at, :updated_by)`
		now := time.Now().UTC()
		for _, rule := range rules {
			payload := map[string]any{
				"name":               strings.TrimSpace(rule.Name),
				"description":        strings.TrimSpace(rule.Description),
				"severities":         toJSON(rule.Severities),
				"channels":           toJSON(rule.Channels),
				"escalation_minutes": rule.EscalationMinutes,
				"enabled":            boolToInt(rule.Enabled),
				"updated_at":         now,
				"updated_by":         actor,
			}
			if _, err := tx.NamedExecContext(ctx, stmt, payload); err != nil {
				return err
			}
		}
		return nil
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func toJSON(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	cleaned := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	data, _ := json.Marshal(cleaned)
	return string(data)
}
