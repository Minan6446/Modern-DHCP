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
    audit_id, tenant_id, actor, action, source, resource, correlation_id, payload, created_at)
VALUES (
	:audit_id, :tenant_id, :actor, :action, :source, :resource, :correlation_id, :payload, :created_at)`, event)
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
WHERE tenant_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, tenantID, limit, offset); err != nil {
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
	query := `SELECT * FROM audit_events WHERE tenant_id = ?`
	args := []any{tenantID}
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
	if filter.Resource != "" {
		query += " AND resource = ?"
		args = append(args, filter.Resource)
	}
	if filter.CorrelationID != "" {
		query += " AND correlation_id = ?"
		args = append(args, filter.CorrelationID)
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	var events []models.AuditEvent
	if err := r.db.SelectContext(ctx, &events, query, args...); err != nil {
		return nil, err
	}
	return events, nil
}
