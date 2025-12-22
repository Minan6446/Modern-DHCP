package collab

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrLockConflict indicates another session already holds the requested lock.
	ErrLockConflict = errors.New("collab: lock conflict")
	// ErrSessionNotFound indicates the referenced session is missing or expired.
	ErrSessionNotFound = errors.New("collab: session not found")
)

// Session captures realtime editing presence for a resource.
type Session struct {
	ID           string          `db:"id" json:"id"`
	TenantID     string          `db:"tenant_id" json:"tenantId"`
	ResourceType string          `db:"resource_type" json:"resourceType"`
	ResourceID   string          `db:"resource_id" json:"resourceId"`
	UserID       string          `db:"user_id" json:"userId"`
	Status       string          `db:"status" json:"status"`
	LockVersion  int             `db:"lock_version" json:"lockVersion"`
	ExpiresAt    time.Time       `db:"expires_at" json:"expiresAt"`
	Metadata     json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updatedAt"`
	DeletedAt    *time.Time      `db:"deleted_at" json:"deletedAt,omitempty"`
	Version      int             `db:"version" json:"version"`
}

// Lock enforces optimistic edits per resource.
type Lock struct {
	ID           string     `db:"id" json:"id"`
	TenantID     string     `db:"tenant_id" json:"tenantId"`
	ResourceType string     `db:"resource_type" json:"resourceType"`
	ResourceID   string     `db:"resource_id" json:"resourceId"`
	SessionID    string     `db:"session_id" json:"sessionId"`
	Status       string     `db:"status" json:"status"`
	AcquiredAt   time.Time  `db:"acquired_at" json:"acquiredAt"`
	ExpiresAt    time.Time  `db:"expires_at" json:"expiresAt"`
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt    *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
	Version      int        `db:"version" json:"version"`
}

// Comment attaches threaded collaboration to resources.
type Comment struct {
	ID           string     `db:"id" json:"id"`
	TenantID     string     `db:"tenant_id" json:"tenantId"`
	ResourceType string     `db:"resource_type" json:"resourceType"`
	ResourceID   string     `db:"resource_id" json:"resourceId"`
	AuthorID     string     `db:"author_id" json:"authorId"`
	ParentID     *string    `db:"parent_id" json:"parentId,omitempty"`
	Body         string     `db:"body" json:"body"`
	Status       string     `db:"status" json:"status"`
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt    *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
	Version      int        `db:"version" json:"version"`
}

// Task coordinates assignments tied to a resource.
type Task struct {
	ID           string          `db:"id" json:"id"`
	TenantID     string          `db:"tenant_id" json:"tenantId"`
	Title        string          `db:"title" json:"title"`
	AssigneeID   *string         `db:"assignee_id" json:"assigneeId,omitempty"`
	ResourceType string          `db:"resource_type" json:"resourceType"`
	ResourceID   string          `db:"resource_id" json:"resourceId"`
	ResourceRef  json.RawMessage `db:"resource_ref" json:"resourceRef"`
	State        string          `db:"state" json:"state"`
	Priority     int             `db:"priority" json:"priority"`
	DueAt        *time.Time      `db:"due_at" json:"dueAt,omitempty"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updatedAt"`
	DeletedAt    *time.Time      `db:"deleted_at" json:"deletedAt,omitempty"`
	Version      int             `db:"version" json:"version"`
}

// Approval documents workflow decisions for change requests.
type Approval struct {
	ID         string     `db:"id" json:"id"`
	TenantID   string     `db:"tenant_id" json:"tenantId"`
	WorkflowID string     `db:"workflow_id" json:"workflowId"`
	Stage      int        `db:"stage" json:"stage"`
	ApproverID string     `db:"approver_id" json:"approverId"`
	Decision   string     `db:"decision" json:"decision"`
	DecidedAt  *time.Time `db:"decided_at" json:"decidedAt,omitempty"`
	Comment    *string    `db:"comment" json:"comment,omitempty"`
	CreatedAt  time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time  `db:"updated_at" json:"updatedAt"`
	DeletedAt  *time.Time `db:"deleted_at" json:"deletedAt,omitempty"`
	Version    int        `db:"version" json:"version"`
}

// Event is an append-only record emitted by the collaboration hub.
type Event struct {
	ID           int64           `db:"id" json:"id"`
	TenantID     string          `db:"tenant_id" json:"tenantId"`
	SessionID    *string         `db:"session_id" json:"sessionId,omitempty"`
	ResourceType *string         `db:"resource_type" json:"resourceType,omitempty"`
	ResourceID   *string         `db:"resource_id" json:"resourceId,omitempty"`
	EventType    string          `db:"event_type" json:"eventType"`
	Payload      json.RawMessage `db:"payload" json:"payload"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
}
