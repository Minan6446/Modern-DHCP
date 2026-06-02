package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// DHCPTemplateApplyHistory captures persisted template operation history.
type DHCPTemplateApplyHistory struct {
	ID                 uint64          `json:"id" db:"id"`
	TemplateID         string          `json:"templateId" db:"template_id"`
	TemplateName       string          `json:"templateName" db:"template_name"`
	Action             string          `json:"action" db:"action"`
	Operator           string          `json:"operator" db:"operator"`
	TargetScope        string          `json:"targetScope" db:"target_scope"`
	AppliedOptionCount int             `json:"appliedOptionCount" db:"applied_option_count"`
	TemplateSnapshot   json.RawMessage `json:"templateSnapshot,omitempty" db:"template_snapshot"`
	CreatedAt          time.Time       `json:"createdAt" db:"created_at"`
}

// TemplateApplyHistoryRepository persists template apply history.
type TemplateApplyHistoryRepository interface {
	Create(ctx context.Context, item DHCPTemplateApplyHistory) error
	ListByTemplate(ctx context.Context, templateID string, limit, offset int) ([]DHCPTemplateApplyHistory, int64, error)
}

// SQLTemplateApplyHistoryRepository implements TemplateApplyHistoryRepository via SQL.
type SQLTemplateApplyHistoryRepository struct {
	db *sqlx.DB
}

// NewSQLTemplateApplyHistoryRepository constructs SQL history repository.
func NewSQLTemplateApplyHistoryRepository(db *sqlx.DB) *SQLTemplateApplyHistoryRepository {
	if db == nil {
		return nil
	}
	return &SQLTemplateApplyHistoryRepository{db: db}
}

func (r *SQLTemplateApplyHistoryRepository) Create(ctx context.Context, item DHCPTemplateApplyHistory) error {
	if r == nil || r.db == nil {
		return nil
	}
	item.TemplateID = strings.TrimSpace(item.TemplateID)
	item.TemplateName = strings.TrimSpace(item.TemplateName)
	item.Action = strings.TrimSpace(item.Action)
	item.Operator = strings.TrimSpace(item.Operator)
	item.TargetScope = strings.TrimSpace(item.TargetScope)
	if item.Action == "" {
		item.Action = "apply"
	}
	if item.Operator == "" {
		item.Operator = "system"
	}
	if item.TargetScope == "" {
		item.TargetScope = "global"
	}
	if item.AppliedOptionCount < 0 {
		item.AppliedOptionCount = 0
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}

	stmt := `INSERT INTO dhcp_option_template_apply_history (template_id, template_name, action, operator, target_scope, applied_option_count, template_snapshot, created_at) VALUES (:template_id, :template_name, :action, :operator, :target_scope, :applied_option_count, :template_snapshot, :created_at)`
	_, err := r.db.NamedExecContext(ctx, stmt, item)
	return err
}

func (r *SQLTemplateApplyHistoryRepository) ListByTemplate(ctx context.Context, templateID string, limit, offset int) ([]DHCPTemplateApplyHistory, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	templateID = strings.TrimSpace(templateID)
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	countStmt := `SELECT COUNT(*) FROM dhcp_option_template_apply_history WHERE template_id = ?`
	countStmt = r.db.Rebind(countStmt)
	var total sql.NullInt64
	if err := r.db.GetContext(ctx, &total, countStmt, templateID); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, template_id, template_name, action, operator, target_scope, applied_option_count, template_snapshot, created_at FROM dhcp_option_template_apply_history WHERE template_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	query = r.db.Rebind(query)
	rows := make([]DHCPTemplateApplyHistory, 0, limit)
	if err := r.db.SelectContext(ctx, &rows, query, templateID, limit, offset); err != nil {
		return nil, 0, err
	}
	totalValue := int64(0)
	if total.Valid {
		totalValue = total.Int64
	}
	return rows, totalValue, nil
}
