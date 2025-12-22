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
	ListAssignments(ctx context.Context, principalID string) ([]Assignment, error)
	ListAssignmentsByTenant(ctx context.Context, principalID, tenantID string) ([]Assignment, error)
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

// ListAssignments fetches all assignments for the principal regardless of tenant.
func (r *MySQLRepository) ListAssignments(ctx context.Context, principalID string) ([]Assignment, error) {
	const query = `
SELECT id, principal_id, role_name, tenant_id, org_unit_id, resource_type, resource_id,
       created_by, created_at, expires_at, attributes
FROM rbac_assignments
WHERE principal_id = ?`
	var assignments []Assignment
	if err := r.db.SelectContext(ctx, &assignments, query, principalID); err != nil {
		return nil, err
	}
	return assignments, nil
}

// ListAssignmentsByTenant fetches assignments filtered by tenant scope.
func (r *MySQLRepository) ListAssignmentsByTenant(ctx context.Context, principalID, tenantID string) ([]Assignment, error) {
	const query = `
SELECT id, principal_id, role_name, tenant_id, org_unit_id, resource_type, resource_id,
       created_by, created_at, expires_at, attributes
FROM rbac_assignments
WHERE principal_id = ?
  AND (tenant_id = ? OR tenant_id IS NULL)
`
	var assignments []Assignment
	if err := r.db.SelectContext(ctx, &assignments, query, principalID, tenantID); err != nil {
		return nil, err
	}
	return assignments, nil
}

// CreateAssignment inserts a new permanent grant.
func (r *MySQLRepository) CreateAssignment(ctx context.Context, assignment *Assignment) error {
	const statement = `
INSERT INTO rbac_assignments (
    id, principal_id, role_name, tenant_id, org_unit_id, resource_type, resource_id,
    created_by, created_at, expires_at, attributes)
VALUES (
    :id, :principal_id, :role_name, :tenant_id, :org_unit_id, :resource_type, :resource_id,
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
