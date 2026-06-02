package rbac

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

// Repository exposes persistence helpers for RBAC data.
type Repository interface {
	GetRole(ctx context.Context, name string) (Role, error)
	ListRoles(ctx context.Context) ([]Role, error)
	CreateRole(ctx context.Context, role Role) error
	UpdateRole(ctx context.Context, originalName string, role Role) error
	DeleteRole(ctx context.Context, name string) error
	ListAssignments(ctx context.Context, principalID string) ([]Assignment, error)
	CreateAssignment(ctx context.Context, assignment *Assignment) error
	DeleteAssignment(ctx context.Context, assignmentID string) error
	CreateTempGrant(ctx context.Context, grant *TempGrant) error
	ListActiveTempGrants(ctx context.Context, principalID string, now time.Time) ([]TempGrant, error)
	UpdateTempGrantStatus(ctx context.Context, grantID, status string, approver *string, approvedAt *time.Time) error
	ListApprovalRules(ctx context.Context, roleName string) ([]ApprovalRule, error)
}

// MySQLRepository stores RBAC artifacts in a MySQL-compatible database.
type MySQLRepository struct {
	db *sqlx.DB
}

// NewRepository constructs a Repository backed by sqlx.
func NewRepository(db *sqlx.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// GetRole fetches a single role definition.
func (r *MySQLRepository) GetRole(ctx context.Context, name string) (Role, error) {
	const query = `
SELECT name, inherits_from, description, capabilities, created_at, updated_at
FROM rbac_roles
WHERE name = ?
LIMIT 1`
	var role Role
	if err := r.db.GetContext(ctx, &role, query, name); err != nil {
		if err == sql.ErrNoRows {
			return Role{}, ErrRoleNotFound
		}
		return Role{}, err
	}
	r.unmarshalRoleCaps(&role)
	return role, nil
}

// ListRoles returns all registered roles.
func (r *MySQLRepository) ListRoles(ctx context.Context) ([]Role, error) {
	const query = `
SELECT name, inherits_from, description, capabilities, created_at, updated_at
FROM rbac_roles
ORDER BY name`
	var roles []Role
	if err := r.db.SelectContext(ctx, &roles, query); err != nil {
		return nil, err
	}
	for i := range roles {
		r.unmarshalRoleCaps(&roles[i])
	}
	return roles, nil
}

// CreateRole inserts a new role definition.
func (r *MySQLRepository) CreateRole(ctx context.Context, role Role) error {
	const statement = `
INSERT INTO rbac_roles (name, inherits_from, description, capabilities, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`
	caps, err := json.Marshal(role.Capabilities)
	if err != nil {
		return err
	}
	createdAt := role.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := role.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	_, err = r.db.ExecContext(ctx, statement, role.Name, role.InheritsFrom, role.Description, caps, createdAt, updatedAt)
	return err
}

// UpdateRole replaces mutable attributes of an existing role identified by originalName.
func (r *MySQLRepository) UpdateRole(ctx context.Context, originalName string, role Role) error {
	const statement = `
UPDATE rbac_roles
SET name = ?, inherits_from = ?, description = ?, capabilities = ?, updated_at = ?
WHERE name = ?`
	caps, err := json.Marshal(role.Capabilities)
	if err != nil {
		return err
	}
	updatedAt := role.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, statement, role.Name, role.InheritsFrom, role.Description, caps, updatedAt, originalName)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRoleNotFound
	}
	return nil
}

// DeleteRole removes a role definition.
func (r *MySQLRepository) DeleteRole(ctx context.Context, name string) error {
	const statement = `DELETE FROM rbac_roles WHERE name = ?`
	result, err := r.db.ExecContext(ctx, statement, name)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRoleNotFound
	}
	return nil
}

// ListAssignments fetches all assignments for the principal regardless of tenant.
func (r *MySQLRepository) ListAssignments(ctx context.Context, principalID string) ([]Assignment, error) {
	const query = `
SELECT id, principal_id, role_name,
	   COALESCE(group_ids, '[]') AS group_ids,
	   COALESCE(labels, '{}') AS labels,
	   group_scope, label_scope,
       org_unit_id, resource_type, resource_id,
	   created_by, created_at, expires_at,
	   COALESCE(attributes, '{}') AS attributes
FROM rbac_assignments
WHERE principal_id = ?`
	var assignments []Assignment
	if err := r.db.SelectContext(ctx, &assignments, query, principalID); err != nil {
		return nil, err
	}
	for i := range assignments {
		assignments[i].HydrateScope()
	}
	return assignments, nil
}

// CreateAssignment inserts a new permanent grant.
func (r *MySQLRepository) CreateAssignment(ctx context.Context, assignment *Assignment) error {
	assignment.PrepareScopeColumns()
	const statement = `
INSERT INTO rbac_assignments (
	id, principal_id, role_name, group_ids, labels, group_scope, label_scope,
    org_unit_id, resource_type, resource_id,
    created_by, created_at, expires_at, attributes)
VALUES (
	:id, :principal_id, :role_name, :group_ids, :labels, :group_scope, :label_scope,
    :org_unit_id, :resource_type, :resource_id,
    :created_by, :created_at, :expires_at, :attributes)`
	_, err := r.db.NamedExecContext(ctx, statement, assignment)
	return err
}

// DeleteAssignment removes a permanent grant.
func (r *MySQLRepository) DeleteAssignment(ctx context.Context, assignmentID string) error {
	const statement = `DELETE FROM rbac_assignments WHERE id = ?`
	result, err := r.db.ExecContext(ctx, statement, assignmentID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrAssignmentNotFound
	}
	return nil
}

// CreateTempGrant records a temporary grant request or approval.
func (r *MySQLRepository) CreateTempGrant(ctx context.Context, grant *TempGrant) error {
	const statement = `
INSERT INTO rbac_temp_grants (
    id, assignment_id, requested_by, reason, status, expires_at,
    approved_by, approved_at, created_at, updated_at)
VALUES (
    :id, :assignment_id, :requested_by, :reason, :status, :expires_at,
    :approved_by, :approved_at, :created_at, :updated_at)`
	_, err := r.db.NamedExecContext(ctx, statement, grant)
	return err
}

// ListActiveTempGrants returns non-expired approved or pending grants for the caller.
func (r *MySQLRepository) ListActiveTempGrants(ctx context.Context, principalID string, now time.Time) ([]TempGrant, error) {
	const query = `
SELECT tg.id, tg.assignment_id, a.principal_id, tg.requested_by, tg.reason, tg.status,
       tg.expires_at, tg.approved_by, tg.approved_at, tg.created_at, tg.updated_at
FROM rbac_temp_grants tg
JOIN rbac_assignments a ON a.id = tg.assignment_id
WHERE a.principal_id = ?
  AND tg.expires_at >= ?`
	var grants []TempGrant
	if err := r.db.SelectContext(ctx, &grants, query, principalID, now); err != nil {
		return nil, err
	}
	return grants, nil
}

// UpdateTempGrantStatus updates approval metadata for a grant.
func (r *MySQLRepository) UpdateTempGrantStatus(ctx context.Context, grantID, status string, approver *string, approvedAt *time.Time) error {
	const statement = `
UPDATE rbac_temp_grants
SET status = ?, approved_by = ?, approved_at = ?, updated_at = ?
WHERE id = ?`
	_, err := r.db.ExecContext(ctx, statement, status, approver, approvedAt, time.Now().UTC(), grantID)
	return err
}

// ListApprovalRules retrieves workflow constraints for a target role.
func (r *MySQLRepository) ListApprovalRules(ctx context.Context, roleName string) ([]ApprovalRule, error) {
	const query = `
SELECT id, role_name, scope, min_approvers, approver_role, created_at, updated_at
FROM rbac_approval_rules
WHERE role_name = ?`
	var rules []ApprovalRule
	if err := r.db.SelectContext(ctx, &rules, query, roleName); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *MySQLRepository) unmarshalRoleCaps(role *Role) {
	if role == nil || len(role.RawCaps) == 0 {
		return
	}
	var caps []string
	if err := json.Unmarshal(role.RawCaps, &caps); err != nil {
		// leave RawCaps for debugging but avoid panics; downstream code can handle nil capabilities
		return
	}
	role.Capabilities = caps
}

var _ Repository = (*MySQLRepository)(nil)
