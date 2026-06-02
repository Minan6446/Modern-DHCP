package audit

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"

	"modern-dhcp/pkg/models"
)

// Repository defines persistence helpers for audit events.
type Repository interface {
	InsertEvent(ctx context.Context, event *models.AuditEvent) error
	ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]models.AuditEvent, error)
	ListEventsFiltered(ctx context.Context, tenantID string, filter ListEventsFilter) ([]models.AuditEvent, error)
	CountEventsFiltered(ctx context.Context, tenantID string, filter ListEventsFilter) (int, error)
}

// MySQLRepository stores audit records in MySQL.
type MySQLRepository struct {
	db *sqlx.DB
}

// NewRepository creates a MySQL-backed audit repository.
func NewRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) InsertEvent(ctx context.Context, event *models.AuditEvent) error {
	_, err := r.db.NamedExecContext(ctx, `
INSERT INTO audit_events (
    audit_id, actor, action, source, resource, correlation_id, payload, created_at)
VALUES (
	:audit_id, :actor, :action, :source, :resource, :correlation_id, :payload, :created_at)`, event)
	return err
}

func (r *MySQLRepository) ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]models.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var events []models.AuditEvent
	if err := r.db.SelectContext(ctx, &events, `
SELECT * FROM audit_events
WHERE 1=1
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, limit, offset); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *MySQLRepository) ListEventsFiltered(ctx context.Context, tenantID string, filter ListEventsFilter) ([]models.AuditEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query := `SELECT * FROM audit_events WHERE 1=1`
	args := []any{}
	if filter.Actor != "" {
		query += " AND actor = ?"
		args = append(args, filter.Actor)
	}
	if len(filter.Actions) > 0 {
		placeholders := strings.Repeat("?,", len(filter.Actions))
		placeholders = placeholders[:len(placeholders)-1]
		query += " AND action IN (" + placeholders + ")"
		for _, action := range filter.Actions {
			args = append(args, action)
		}
	}
	if len(filter.ActionPrefixes) > 0 {
		query += " AND ("
		for i, prefix := range filter.ActionPrefixes {
			if i > 0 {
				query += " OR "
			}
			query += "action LIKE ?"
			args = append(args, strings.TrimSpace(prefix)+"%")
		}
		query += ")"
	}
	if len(filter.ExcludeActions) > 0 {
		placeholders := strings.Repeat("?,", len(filter.ExcludeActions))
		placeholders = placeholders[:len(placeholders)-1]
		query += " AND action NOT IN (" + placeholders + ")"
		for _, action := range filter.ExcludeActions {
			args = append(args, action)
		}
	}
	if filter.Resource != "" {
		query += " AND resource = ?"
		args = append(args, filter.Resource)
	}
	if filter.CorrelationID != "" {
		query += " AND correlation_id = ?"
		args = append(args, filter.CorrelationID)
	}
	if filter.StartAt != nil {
		query += " AND created_at >= ?"
		args = append(args, filter.StartAt)
	}
	if filter.EndAt != nil {
		query += " AND created_at <= ?"
		args = append(args, filter.EndAt)
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	var events []models.AuditEvent
	if err := r.db.SelectContext(ctx, &events, query, args...); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *MySQLRepository) CountEventsFiltered(ctx context.Context, tenantID string, filter ListEventsFilter) (int, error) {
	query := `SELECT COUNT(*) FROM audit_events WHERE 1=1`
	args := []any{}
	if filter.Actor != "" {
		query += " AND actor = ?"
		args = append(args, filter.Actor)
	}
	if len(filter.Actions) > 0 {
		placeholders := strings.Repeat("?,", len(filter.Actions))
		placeholders = placeholders[:len(placeholders)-1]
		query += " AND action IN (" + placeholders + ")"
		for _, action := range filter.Actions {
			args = append(args, action)
		}
	}
	if len(filter.ActionPrefixes) > 0 {
		query += " AND ("
		for i, prefix := range filter.ActionPrefixes {
			if i > 0 {
				query += " OR "
			}
			query += "action LIKE ?"
			args = append(args, strings.TrimSpace(prefix)+"%")
		}
		query += ")"
	}
	if len(filter.ExcludeActions) > 0 {
		placeholders := strings.Repeat("?,", len(filter.ExcludeActions))
		placeholders = placeholders[:len(placeholders)-1]
		query += " AND action NOT IN (" + placeholders + ")"
		for _, action := range filter.ExcludeActions {
			args = append(args, action)
		}
	}
	if filter.Resource != "" {
		query += " AND resource = ?"
		args = append(args, filter.Resource)
	}
	if filter.CorrelationID != "" {
		query += " AND correlation_id = ?"
		args = append(args, filter.CorrelationID)
	}
	if filter.StartAt != nil {
		query += " AND created_at >= ?"
		args = append(args, filter.StartAt)
	}
	if filter.EndAt != nil {
		query += " AND created_at <= ?"
		args = append(args, filter.EndAt)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, query, args...); err != nil {
		return 0, err
	}
	return total, nil
}
