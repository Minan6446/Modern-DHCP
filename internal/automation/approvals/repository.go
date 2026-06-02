package approvals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// ErrRequestNotFound is returned when a change request cannot be located.
var ErrRequestNotFound = errors.New("automation: change request not found")

// ListOptions scopes change request listings.
type ListOptions struct {
	TenantID string
	JobTypes []string
	Statuses []RequestStatus
	Limit    int
	Offset   int
}

// Repository persists automation change requests.
type Repository interface {
	Create(ctx context.Context, request *ChangeRequest) error
	Get(ctx context.Context, id string) (*ChangeRequest, error)
	List(ctx context.Context, opts ListOptions) ([]ChangeRequest, error)
	UpdateDecision(ctx context.Context, id, approver, note string, status RequestStatus, decidedAt time.Time) (*ChangeRequest, error)
	MarkApplied(ctx context.Context, id string, appliedAt time.Time) error
}

// SQLRepository provides a MySQL-compatible implementation.
type SQLRepository struct {
	db *sqlx.DB
}

// NewRepository builds a repository backed by sqlx.
func NewRepository(db *sqlx.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

var _ Repository = (*SQLRepository)(nil)

// Create inserts a new change request row.
func (r *SQLRepository) Create(ctx context.Context, request *ChangeRequest) error {
	if request == nil {
		return errors.New("automation: request required")
	}
	now := time.Now().UTC()
	if request.CreatedAt.IsZero() {
		request.CreatedAt = now
	}
	if request.UpdatedAt.IsZero() {
		request.UpdatedAt = request.CreatedAt
	}
	if request.Status == "" {
		request.Status = RequestStatusPending
	}
	if request.Version <= 0 {
		request.Version = 1
	}
	const query = `
INSERT INTO automation_change_requests (
	id, job_type, request_type, status,
    requested_by, requested_at, approver_id, decided_at, decision_note,
    original_config, proposed_config, payload, auto_applied, applied_at,
    created_at, updated_at, version)
VALUES (
	:id, :job_type, :request_type, :status,
    :requested_by, :requested_at, :approver_id, :decided_at, :decision_note,
    :original_config, :proposed_config, :payload, :auto_applied, :applied_at,
    :created_at, :updated_at, :version)`
	_, err := r.db.NamedExecContext(ctx, query, request)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return fmt.Errorf("automation: change request already exists: %w", err)
		}
		return err
	}
	return nil
}

// Get retrieves a change request by identifier.
func (r *SQLRepository) Get(ctx context.Context, id string) (*ChangeRequest, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("automation: change request id required")
	}
	const query = `
SELECT id, job_type, request_type, status,
       requested_by, requested_at, approver_id, decided_at, decision_note,
       original_config, proposed_config, payload, auto_applied, applied_at,
       created_at, updated_at, version
FROM automation_change_requests
WHERE id = ?`
	var row ChangeRequest
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRequestNotFound
		}
		return nil, err
	}
	clone := row.Clone()
	return &clone, nil
}

// List loads change requests filtered by tenant/job type/status.
func (r *SQLRepository) List(ctx context.Context, opts ListOptions) ([]ChangeRequest, error) {
	query := `
SELECT id, job_type, request_type, status,
       requested_by, requested_at, approver_id, decided_at, decision_note,
       original_config, proposed_config, payload, auto_applied, applied_at,
       created_at, updated_at, version
FROM automation_change_requests
WHERE 1 = 1`
	args := make([]any, 0)
	if len(opts.JobTypes) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(opts.JobTypes)), ",")
		query += " AND job_type IN (" + placeholders + ")"
		for _, jobType := range opts.JobTypes {
			args = append(args, jobType)
		}
	}
	if len(opts.Statuses) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(opts.Statuses)), ",")
		query += " AND status IN (" + placeholders + ")"
		for _, status := range opts.Statuses {
			args = append(args, status)
		}
	}
	query += " ORDER BY requested_at DESC"
	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query += " LIMIT ?"
	args = append(args, limit)
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}
	var rows []ChangeRequest
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	results := make([]ChangeRequest, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.Clone())
	}
	return results, nil
}

// UpdateDecision records an approval or rejection.
func (r *SQLRepository) UpdateDecision(ctx context.Context, id, approver, note string, status RequestStatus, decidedAt time.Time) (*ChangeRequest, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("automation: change request id required")
	}
	if strings.TrimSpace(approver) == "" {
		return nil, errors.New("automation: approver required")
	}
	if status != RequestStatusApproved && status != RequestStatusRejected {
		return nil, errors.New("automation: invalid decision status")
	}
	now := decidedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := r.db.NamedExecContext(ctx, `
UPDATE automation_change_requests
SET status = :status,
    approver_id = :approver_id,
    decided_at = :decided_at,
    decision_note = :decision_note,
    updated_at = :updated_at,
    version = version + 1
WHERE id = :id AND status = 'pending'`, map[string]any{
		"id":            id,
		"status":        status,
		"approver_id":   strings.TrimSpace(approver),
		"decided_at":    now,
		"decision_note": strings.TrimSpace(note),
		"updated_at":    now,
	})
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrRequestNotFound
	}
	return r.Get(ctx, id)
}

// MarkApplied stamps the request as applied once the change is executed.
func (r *SQLRepository) MarkApplied(ctx context.Context, id string, appliedAt time.Time) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("automation: change request id required")
	}
	now := appliedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res, err := r.db.NamedExecContext(ctx, `
UPDATE automation_change_requests
SET status = CASE WHEN status = 'approved' THEN 'applied' ELSE status END,
    auto_applied = 1,
    applied_at = :applied_at,
    updated_at = :updated_at,
    version = version + 1
WHERE id = :id`, map[string]any{
		"id":         id,
		"applied_at": now,
		"updated_at": now,
	})
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRequestNotFound
	}
	return nil
}
