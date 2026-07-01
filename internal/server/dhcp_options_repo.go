package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// OptionRepository persists custom DHCP options.
type OptionRepository interface {
	List(ctx context.Context, tenantID string) ([]dhcpOptionTemplate, error)
	Upsert(ctx context.Context, opt dhcpOptionTemplate, tenantID string) (dhcpOptionTemplate, error)
	Delete(ctx context.Context, optionID string, tenantID string) error
}

// SQLOptionRepository implements OptionRepository using a SQL database.
type SQLOptionRepository struct {
	db *sqlx.DB
}

// NewSQLOptionRepository constructs a SQL-backed option repository.
func NewSQLOptionRepository(db *sqlx.DB) *SQLOptionRepository {
	if db == nil {
		return nil
	}
	return &SQLOptionRepository{db: db}
}

type optionRecord struct {
	ID            string         `db:"id"`
	Code          int            `db:"code"`
	Name          string         `db:"name"`
	Scope         string         `db:"scope"`
	Format        string         `db:"format"`
	DataType      string         `db:"data_type"`
	Value         string         `db:"value"`
	ValueExample  sql.NullString `db:"value_example"`
	AllowedValues sql.NullString `db:"allowed_values"`
	SampleValue   sql.NullString `db:"sample_value"`
	Description   sql.NullString `db:"description"`
	Tags          sql.NullString `db:"tags"`
	CreatedAt     time.Time      `db:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at"`
}

func encodeStrings(values []string) string {
	if len(values) == 0 {
		return ""
	}
	b, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(b)
}

func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func decodeStrings(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func (r *SQLOptionRepository) List(ctx context.Context, tenantID string) ([]dhcpOptionTemplate, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	query := `SELECT id, code, name, scope, format, data_type, value, value_example, allowed_values, sample_value, description, tags, created_at, updated_at FROM dhcp_custom_options ORDER BY code ASC, name ASC`
	query = r.db.Rebind(query)
	var rows []optionRecord
	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	out := make([]dhcpOptionTemplate, 0, len(rows))
	for _, rec := range rows {
		out = append(out, dhcpOptionTemplate{
			ID:            rec.ID,
			Name:          rec.Name,
			Code:          rec.Code,
			Scope:         rec.Scope,
			Format:        rec.Format,
			DataType:      rec.DataType,
			Value:         rec.Value,
			ValueExample:  rec.ValueExample.String,
			AllowedValues: decodeStrings(rec.AllowedValues.String),
			SampleValue:   rec.SampleValue.String,
			Description:   rec.Description.String,
			Tags:          decodeStrings(rec.Tags.String),
			UpdatedAt:     rec.UpdatedAt,
		})
	}
	return out, nil
}

func (r *SQLOptionRepository) Upsert(ctx context.Context, opt dhcpOptionTemplate, tenantID string) (dhcpOptionTemplate, error) {
	if r == nil || r.db == nil {
		return opt, nil
	}
	now := time.Now().UTC()
	if strings.TrimSpace(opt.ID) == "" {
		opt.ID = uuid.NewString()
	}
	rec := optionRecord{
		ID:            opt.ID,
		Code:          opt.Code,
		Name:          opt.Name,
		Scope:         opt.Scope,
		Format:        opt.Format,
		DataType:      opt.DataType,
		Value:         opt.Value,
		ValueExample:  toNullString(opt.ValueExample),
		AllowedValues: toNullString(encodeStrings(opt.AllowedValues)),
		SampleValue:   toNullString(opt.SampleValue),
		Description:   toNullString(opt.Description),
		Tags:          toNullString(encodeStrings(opt.Tags)),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	updateStmt := `UPDATE dhcp_custom_options SET code=:code, name=:name, scope=:scope, format=:format, data_type=:data_type, value=:value, value_example=:value_example, allowed_values=:allowed_values, sample_value=:sample_value, description=:description, tags=:tags, updated_at=:updated_at WHERE id=:id`
	res, err := r.db.NamedExecContext(ctx, updateStmt, rec)
	if err == nil {
		if rows, _ := res.RowsAffected(); rows > 0 {
			opt.UpdatedAt = now
			return opt, nil
		}
	}

	insertStmt := `INSERT INTO dhcp_custom_options (id, code, name, scope, format, data_type, value, value_example, allowed_values, sample_value, description, tags, created_at, updated_at) VALUES (:id, :code, :name, :scope, :format, :data_type, :value, :value_example, :allowed_values, :sample_value, :description, :tags, :created_at, :updated_at)`
	if _, err := r.db.NamedExecContext(ctx, insertStmt, rec); err != nil {
		return opt, err
	}
	opt.UpdatedAt = now
	return opt, nil
}

func (r *SQLOptionRepository) Delete(ctx context.Context, optionID string, tenantID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	stmt := `DELETE FROM dhcp_custom_options WHERE id = ?`
	stmt = r.db.Rebind(stmt)
	_, err := r.db.ExecContext(ctx, stmt, optionID)
	return err
}
