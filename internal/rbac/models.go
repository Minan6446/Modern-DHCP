package rbac

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrRoleNotFound indicates the requested role record does not exist.
	ErrRoleNotFound = errors.New("rbac: role not found")
	// ErrAssignmentNotFound is returned when an assignment lookup fails.
	ErrAssignmentNotFound = errors.New("rbac: assignment not found")
	// ErrTempGrantNotFound signals that the temporary grant record cannot be located.
	ErrTempGrantNotFound = errors.New("rbac: temp grant not found")
)

// Role models a named RBAC role along with its inherited parent and capability set.
type Role struct {
	Name         string          `db:"name" json:"name"`
	InheritsFrom *string         `db:"inherits_from" json:"inheritsFrom"`
	Description  string          `db:"description" json:"description"`
	Capabilities []string        `db:"-" json:"capabilities"`
	RawCaps      json.RawMessage `db:"capabilities" json:"-"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time       `db:"updated_at" json:"updatedAt"`
}

// OrgUnit represents a node in the organizational hierarchy used for permission inheritance.
type OrgUnit struct {
	ID        string     `db:"id" json:"id"`
	TenantID  string     `db:"tenant_id" json:"tenantId"`
	ParentID  *string    `db:"parent_id" json:"parentId"`
	Name      string     `db:"name" json:"name"`
	Path      *string    `db:"path" json:"path"`
	CreatedAt time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time  `db:"updated_at" json:"updatedAt"`
	Children  []*OrgUnit `db:"-" json:"children,omitempty"`
}

// Assignment permanently binds a principal to a role within an optional tenant/org scope.
type Assignment struct {
	ID           string          `db:"id" json:"id"`
	PrincipalID  string          `db:"principal_id" json:"principalId"`
	RoleName     string          `db:"role_name" json:"roleName"`
	TenantID     *string         `db:"tenant_id" json:"tenantId,omitempty"`
	OrgUnitID    *string         `db:"org_unit_id" json:"orgUnitId,omitempty"`
	ResourceType *string         `db:"resource_type" json:"resourceType,omitempty"`
	ResourceID   *string         `db:"resource_id" json:"resourceId,omitempty"`
	CreatedBy    string          `db:"created_by" json:"createdBy"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
	ExpiresAt    *time.Time      `db:"expires_at" json:"expiresAt,omitempty"`
	Attributes   json.RawMessage `db:"attributes" json:"attributes,omitempty"`
}

// TempGrant expresses a time-boxed elevation request linked to an assignment template.
type TempGrant struct {
	ID           string     `db:"id" json:"id"`
	AssignmentID string     `db:"assignment_id" json:"assignmentId"`
	PrincipalID  string     `db:"principal_id" json:"principalId"`
	RequestedBy  string     `db:"requested_by" json:"requestedBy"`
	Reason       string     `db:"reason" json:"reason"`
	Status       string     `db:"status" json:"status"`
	ExpiresAt    time.Time  `db:"expires_at" json:"expiresAt"`
	ApprovedBy   *string    `db:"approved_by" json:"approvedBy,omitempty"`
	ApprovedAt   *time.Time `db:"approved_at" json:"approvedAt,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
}

// ApprovalRule defines workflow requirements before a role grant becomes active.
type ApprovalRule struct {
	ID           string    `db:"id" json:"id"`
	RoleName     string    `db:"role_name" json:"roleName"`
	Scope        string    `db:"scope" json:"scope"`
	MinApprovers int       `db:"min_approvers" json:"minApprovers"`
	ApproverRole string    `db:"approver_role" json:"approverRole"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
}

// Resolution represents the computed capability set for a principal within a context.
type Resolution struct {
	PrincipalID  string
	TenantID     string
	OrgUnitID    string
	Capabilities map[string]struct{}
	ExpiresAt    *time.Time
	Grants       []GrantEnvelope
}

// GrantEnvelope shows which assignment or temporary grant yielded a capability.
type GrantEnvelope struct {
	Assignment   Assignment
	TempGrant    *TempGrant
	Capabilities []string
}

// HasCapability reports whether the resolved set contains the given capability.
func (r Resolution) HasCapability(value string) bool {
	if value == "" {
		return true
	}
	_, ok := r.Capabilities[value]
	return ok
}
