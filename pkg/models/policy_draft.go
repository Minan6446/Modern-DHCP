package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// PolicyDraft captures an unpublished set of rules awaiting promotion.
type PolicyDraft struct {
	ID          string          `db:"id" json:"id"`
	TenantID    string          `db:"tenant_id" json:"tenantId"`
	Name        string          `db:"name" json:"name"`
	Description sql.NullString  `db:"description" json:"description,omitempty"`
	Status      string          `db:"status" json:"status"`
	Rules       json.RawMessage `db:"rules" json:"rules"`
	Metadata    json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	CreatedBy   string          `db:"created_by" json:"createdBy"`
	UpdatedBy   string          `db:"updated_by" json:"updatedBy"`
	CreatedAt   time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time       `db:"updated_at" json:"updatedAt"`
	PublishedAt sql.NullTime    `db:"published_at" json:"publishedAt,omitempty"`
}

// PolicyVersion records a published snapshot of policy rules for rollback and auditing.
type PolicyVersion struct {
	ID          string          `db:"id" json:"id"`
	TenantID    string          `db:"tenant_id" json:"tenantId"`
	Version     int             `db:"version" json:"version"`
	DerivedFrom sql.NullString  `db:"derived_from" json:"derivedFrom,omitempty"`
	Changelog   sql.NullString  `db:"changelog" json:"changelog,omitempty"`
	Rules       json.RawMessage `db:"rules" json:"rules"`
	Metadata    json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	PublishedBy string          `db:"published_by" json:"publishedBy"`
	PublishedAt time.Time       `db:"published_at" json:"publishedAt"`
	RollbackOf  sql.NullString  `db:"rollback_of" json:"rollbackOf,omitempty"`
	CreatedAt   time.Time       `db:"created_at" json:"createdAt"`
}
