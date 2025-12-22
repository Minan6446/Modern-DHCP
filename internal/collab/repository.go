package collab

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// Repository exposes persistence helpers for collaboration artifacts.
type Repository interface {
	UpsertSession(ctx context.Context, session *Session) error
	TouchSession(ctx context.Context, sessionID string, expiresAt time.Time, status string) error
	ListSessions(ctx context.Context, tenantID, resourceType, resourceID string, limit int) ([]Session, error)
	DeleteSession(ctx context.Context, sessionID string) error

	AcquireLock(ctx context.Context, lock *Lock) error
	ReleaseLock(ctx context.Context, lockID string) error
	ReleaseLockForResource(ctx context.Context, tenantID, resourceType, resourceID, sessionID string) error
	ListLocks(ctx context.Context, tenantID, resourceType, resourceID string) ([]Lock, error)

	CreateComment(ctx context.Context, comment *Comment) error
	ListComments(ctx context.Context, tenantID, resourceType, resourceID string, limit int) ([]Comment, error)

	CreateTask(ctx context.Context, task *Task) error
	UpdateTaskState(ctx context.Context, taskID, state string) error
	ListTasks(ctx context.Context, tenantID, resourceType, resourceID string, limit int) ([]Task, error)

	CreateApproval(ctx context.Context, approval *Approval) error
	ListApprovals(ctx context.Context, workflowID string) ([]Approval, error)

	RecordEvent(ctx context.Context, event *Event) error
}

// MySQLRepository persists collaboration records in MySQL-compatible stores.
type MySQLRepository struct {
	db *sqlx.DB
}

// NewRepository builds a Repository backed by sqlx.
func NewRepository(db *sqlx.DB) Repository {
	return &MySQLRepository{db: db}
}

var _ Repository = (*MySQLRepository)(nil)

// UpsertSession inserts or updates a session heartbeat row.
func (r *MySQLRepository) UpsertSession(ctx context.Context, session *Session) error {
	if session == nil {
		return nil
	}
	const query = `
INSERT INTO collab_sessions (
	id, tenant_id, resource_type, resource_id, user_id,
	status, lock_version, expires_at, metadata, created_at, updated_at, deleted_at, version)
VALUES (
	:id, :tenant_id, :resource_type, :resource_id, :user_id,
	:status, :lock_version, :expires_at, :metadata, :created_at, :updated_at, :deleted_at, :version)
ON DUPLICATE KEY UPDATE
	status = VALUES(status),
	expires_at = VALUES(expires_at),
	metadata = VALUES(metadata),
	lock_version = lock_version + 1,
	updated_at = VALUES(updated_at),
	deleted_at = NULL,
	version = version + 1`
	_, err := r.db.NamedExecContext(ctx, query, session)
	return err
}

// TouchSession advances expiry for the session, returning ErrSessionNotFound when missing.
func (r *MySQLRepository) TouchSession(ctx context.Context, sessionID string, expiresAt time.Time, status string) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
UPDATE collab_sessions
SET expires_at = ?, status = ?, lock_version = lock_version + 1, updated_at = ?, version = version + 1
WHERE id = ? AND deleted_at IS NULL`, expiresAt, status, now, sessionID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// ListSessions returns active sessions for a resource.
func (r *MySQLRepository) ListSessions(ctx context.Context, tenantID, resourceType, resourceID string, limit int) ([]Session, error) {
	const query = `
SELECT *
FROM collab_sessions
WHERE tenant_id = ?
	AND resource_type = ?
	AND resource_id = ?
	AND deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT ?`
	var sessions []Session
	if err := r.db.SelectContext(ctx, &sessions, query, tenantID, resourceType, resourceID, limitOrDefault(limit)); err != nil {
		return nil, err
	}
	return sessions, nil
}

// DeleteSession performs a soft delete so history can be audited later.
func (r *MySQLRepository) DeleteSession(ctx context.Context, sessionID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE collab_sessions
SET deleted_at = ?, status = 'ended', updated_at = ?, version = version + 1
WHERE id = ? AND deleted_at IS NULL`, now, now, sessionID)
	return err
}

// AcquireLock stores a resource lock owned by a session.
func (r *MySQLRepository) AcquireLock(ctx context.Context, lock *Lock) error {
	if lock == nil {
		return nil
	}
	exists, err := r.sessionExists(ctx, lock.SessionID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrSessionNotFound
	}
	const query = `
INSERT INTO collab_locks (
	id, tenant_id, resource_type, resource_id, session_id,
	status, acquired_at, expires_at, created_at, updated_at, deleted_at, version)
VALUES (
	:id, :tenant_id, :resource_type, :resource_id, :session_id,
	:status, :acquired_at, :expires_at, :created_at, :updated_at, :deleted_at, :version)`
	if _, err := r.db.NamedExecContext(ctx, query, lock); err != nil {
		if isDuplicateErr(err) {
			return ErrLockConflict
		}
		return err
	}
	return nil
}

// ReleaseLock marks a lock as deleted by its primary key.
func (r *MySQLRepository) ReleaseLock(ctx context.Context, lockID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE collab_locks
SET deleted_at = ?, status = 'released', updated_at = ?, version = version + 1
WHERE id = ? AND deleted_at IS NULL`, now, now, lockID)
	return err
}

// ReleaseLockForResource clears a lock when session disconnects.
func (r *MySQLRepository) ReleaseLockForResource(ctx context.Context, tenantID, resourceType, resourceID, sessionID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE collab_locks
SET deleted_at = ?, status = 'released', updated_at = ?, version = version + 1
WHERE tenant_id = ? AND resource_type = ? AND resource_id = ? AND session_id = ? AND deleted_at IS NULL`,
		now, tenantID, resourceType, resourceID, sessionID)
	return err
}

// ListLocks returns current locks for the resource (at most 1 due to uniqueness).
func (r *MySQLRepository) ListLocks(ctx context.Context, tenantID, resourceType, resourceID string) ([]Lock, error) {
	const query = `
SELECT *
FROM collab_locks
WHERE tenant_id = ?
	AND resource_type = ?
	AND resource_id = ?
	AND deleted_at IS NULL`
	var locks []Lock
	if err := r.db.SelectContext(ctx, &locks, query, tenantID, resourceType, resourceID); err != nil {
		return nil, err
	}
	return locks, nil
}

// CreateComment inserts a threaded comment.
func (r *MySQLRepository) CreateComment(ctx context.Context, comment *Comment) error {
	if comment == nil {
		return nil
	}
	const query = `
INSERT INTO collab_comments (
	id, tenant_id, resource_type, resource_id, author_id,
	parent_id, body, status, created_at, updated_at, deleted_at, version)
VALUES (
	:id, :tenant_id, :resource_type, :resource_id, :author_id,
	:parent_id, :body, :status, :created_at, :updated_at, :deleted_at, :version)`
	_, err := r.db.NamedExecContext(ctx, query, comment)
	return err
}

// ListComments fetches recent comments for the resource.
func (r *MySQLRepository) ListComments(ctx context.Context, tenantID, resourceType, resourceID string, limit int) ([]Comment, error) {
	const query = `
SELECT *
FROM collab_comments
WHERE tenant_id = ?
	AND resource_type = ?
	AND resource_id = ?
	AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT ?`
	var comments []Comment
	if err := r.db.SelectContext(ctx, &comments, query, tenantID, resourceType, resourceID, limitOrDefault(limit)); err != nil {
		return nil, err
	}
	return comments, nil
}

// CreateTask inserts a collaboration task.
func (r *MySQLRepository) CreateTask(ctx context.Context, task *Task) error {
	if task == nil {
		return nil
	}
	const query = `
INSERT INTO collab_tasks (
	id, tenant_id, title, assignee_id, resource_type, resource_id,
	resource_ref, state, priority, due_at, created_at, updated_at, deleted_at, version)
VALUES (
	:id, :tenant_id, :title, :assignee_id, :resource_type, :resource_id,
	:resource_ref, :state, :priority, :due_at, :created_at, :updated_at, :deleted_at, :version)`
	_, err := r.db.NamedExecContext(ctx, query, task)
	return err
}

// UpdateTaskState updates the workflow state of an existing task.
func (r *MySQLRepository) UpdateTaskState(ctx context.Context, taskID, state string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
UPDATE collab_tasks
SET state = ?, updated_at = ?, version = version + 1
WHERE id = ? AND deleted_at IS NULL`, state, now, taskID)
	return err
}

// ListTasks returns active tasks for the resource ordered by priority then recency.
func (r *MySQLRepository) ListTasks(ctx context.Context, tenantID, resourceType, resourceID string, limit int) ([]Task, error) {
	const query = `
SELECT *
FROM collab_tasks
WHERE tenant_id = ?
	AND resource_type = ?
	AND resource_id = ?
	AND deleted_at IS NULL
ORDER BY priority ASC, created_at ASC
LIMIT ?`
	var tasks []Task
	if err := r.db.SelectContext(ctx, &tasks, query, tenantID, resourceType, resourceID, limitOrDefault(limit)); err != nil {
		return nil, err
	}
	return tasks, nil
}

// CreateApproval records an approver decision placeholder.
func (r *MySQLRepository) CreateApproval(ctx context.Context, approval *Approval) error {
	if approval == nil {
		return nil
	}
	const query = `
INSERT INTO collab_approvals (
	id, tenant_id, workflow_id, stage, approver_id,
	decision, decided_at, comment, created_at, updated_at, deleted_at, version)
VALUES (
	:id, :tenant_id, :workflow_id, :stage, :approver_id,
	:decision, :decided_at, :comment, :created_at, :updated_at, :deleted_at, :version)`
	_, err := r.db.NamedExecContext(ctx, query, approval)
	return err
}

// ListApprovals fetches approvals for a workflow ordered by stage.
func (r *MySQLRepository) ListApprovals(ctx context.Context, workflowID string) ([]Approval, error) {
	const query = `
SELECT *
FROM collab_approvals
WHERE workflow_id = ?
	AND deleted_at IS NULL
ORDER BY stage ASC, created_at ASC`
	var approvals []Approval
	if err := r.db.SelectContext(ctx, &approvals, query, workflowID); err != nil {
		return nil, err
	}
	return approvals, nil
}

// RecordEvent appends a hub event for later analytics.
func (r *MySQLRepository) RecordEvent(ctx context.Context, event *Event) error {
	if event == nil {
		return nil
	}
	const query = `
INSERT INTO collab_events (
	tenant_id, session_id, resource_type, resource_id, event_type, payload, created_at)
VALUES (
	:tenant_id, :session_id, :resource_type, :resource_id, :event_type, :payload, :created_at)`
	result, err := r.db.NamedExecContext(ctx, query, event)
	if err != nil {
		return err
	}
	if id, err := result.LastInsertId(); err == nil {
		event.ID = id
	}
	return nil
}

func (r *MySQLRepository) sessionExists(ctx context.Context, sessionID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `
SELECT EXISTS(
	SELECT 1 FROM collab_sessions WHERE id = ? AND deleted_at IS NULL
)`, sessionID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func isDuplicateErr(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}

func limitOrDefault(limit int) int {
	if limit <= 0 || limit > 500 {
		return 100
	}
	return limit
}
