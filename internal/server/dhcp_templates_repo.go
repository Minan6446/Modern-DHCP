package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// TemplateRepository persists reusable DHCP option templates.
type TemplateRepository interface {
	List(ctx context.Context, tenantID string) ([]dhcpConfigTemplate, error)
	Upsert(ctx context.Context, tpl dhcpConfigTemplate, tenantID string) (dhcpConfigTemplate, error)
	Delete(ctx context.Context, templateID string, tenantID string) error
}

// SQLTemplateRepository implements TemplateRepository using SQL.
type SQLTemplateRepository struct {
	db *sqlx.DB
}

// NewSQLTemplateRepository constructs a SQL-backed template repository.
func NewSQLTemplateRepository(db *sqlx.DB) *SQLTemplateRepository {
	if db == nil {
		return nil
	}
	return &SQLTemplateRepository{db: db}
}

type templateRecord struct {
	ID          string    `db:"id"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	Icon        string    `db:"icon"`
	Options     string    `db:"options"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func encodeTemplateOptions(options []dhcpOptionTemplate) string {
	if len(options) == 0 {
		return "[]"
	}
	b, err := json.Marshal(options)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeTemplateOptions(raw string) []dhcpOptionTemplate {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []dhcpOptionTemplate
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func (r *SQLTemplateRepository) List(ctx context.Context, tenantID string) ([]dhcpConfigTemplate, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	query := `SELECT id, name, description, icon, options, created_at, updated_at FROM dhcp_option_templates ORDER BY updated_at DESC, name ASC`
	query = r.db.Rebind(query)
	var rows []templateRecord
	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	out := make([]dhcpConfigTemplate, 0, len(rows))
	for _, rec := range rows {
		options := decodeTemplateOptions(rec.Options)
		normalized := make([]dhcpOptionTemplate, 0, len(options))
		for _, opt := range options {
			normalized = append(normalized, normalizeTemplateOption(opt))
		}
		out = append(out, dhcpConfigTemplate{
			ID:          rec.ID,
			Name:        rec.Name,
			Description: rec.Description,
			Icon:        strings.TrimSpace(rec.Icon),
			Options:     normalized,
			UpdatedAt:   rec.UpdatedAt,
		})
	}
	return out, nil
}

func (r *SQLTemplateRepository) Upsert(ctx context.Context, tpl dhcpConfigTemplate, tenantID string) (dhcpConfigTemplate, error) {
	if r == nil || r.db == nil {
		return tpl, nil
	}
	now := time.Now().UTC()
	if strings.TrimSpace(tpl.ID) == "" {
		tpl.ID = uuid.NewString()
	}
	tpl.Name = strings.TrimSpace(tpl.Name)
	tpl.Description = strings.TrimSpace(tpl.Description)
	tpl.Icon = strings.TrimSpace(tpl.Icon)
	tpl.UpdatedAt = now

	normalized := make([]dhcpOptionTemplate, 0, len(tpl.Options))
	for _, opt := range tpl.Options {
		normalized = append(normalized, normalizeTemplateOption(opt))
	}
	tpl.Options = normalized

	rec := templateRecord{
		ID:          tpl.ID,
		Name:        tpl.Name,
		Description: tpl.Description,
		Icon:        tpl.Icon,
		Options:     encodeTemplateOptions(tpl.Options),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	updateStmt := `UPDATE dhcp_option_templates SET name=:name, description=:description, icon=:icon, options=:options, updated_at=:updated_at WHERE id=:id`
	res, err := r.db.NamedExecContext(ctx, updateStmt, rec)
	if err == nil {
		if rows, _ := res.RowsAffected(); rows > 0 {
			return tpl, nil
		}
	}

	insertStmt := `INSERT INTO dhcp_option_templates (id, name, description, icon, options, created_at, updated_at) VALUES (:id, :name, :description, :icon, :options, :created_at, :updated_at)`
	if _, err := r.db.NamedExecContext(ctx, insertStmt, rec); err != nil {
		return tpl, err
	}
	return tpl, nil
}

func (r *SQLTemplateRepository) Delete(ctx context.Context, templateID string, tenantID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	stmt := `DELETE FROM dhcp_option_templates WHERE id = ?`
	stmt = r.db.Rebind(stmt)
	_, err := r.db.ExecContext(ctx, stmt, templateID)
	return err
}
