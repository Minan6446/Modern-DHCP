package rbac

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"modern-dhcp/internal/resource"
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
	ParentID  *string    `db:"parent_id" json:"parentId"`
	Name      string     `db:"name" json:"name"`
	Path      *string    `db:"path" json:"path"`
	CreatedAt time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time  `db:"updated_at" json:"updatedAt"`
	Children  []*OrgUnit `db:"-" json:"children,omitempty"`
}

// AssignmentScope captures the optional isolation fields bound to an assignment.
type AssignmentScope struct {
	GroupIDs []string          `json:"groupIds,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// AssignmentScopeOption mutates the scope during construction.
type AssignmentScopeOption func(*AssignmentScope)

// NewAssignmentScope builds a normalized scope object.
func NewAssignmentScope(opts ...AssignmentScopeOption) AssignmentScope {
	scope := AssignmentScope{}
	for _, opt := range opts {
		if opt != nil {
			opt(&scope)
		}
	}
	scope.GroupIDs = normalizeAssignmentSlice(scope.GroupIDs)
	scope.Labels = normalizeAssignmentLabels(scope.Labels)
	return scope
}

// WithAssignmentGroups enumerates the groups tied to the assignment.
func WithAssignmentGroups(groups ...string) AssignmentScopeOption {
	return func(scope *AssignmentScope) {
		scope.GroupIDs = append(scope.GroupIDs, groups...)
	}
}

// WithAssignmentLabel records a single label pair onto the scope.
func WithAssignmentLabel(key, value string) AssignmentScopeOption {
	return func(scope *AssignmentScope) {
		if scope.Labels == nil {
			scope.Labels = make(map[string]string)
		}
		scope.Labels[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

// WithAssignmentLabels merges multiple labels.
func WithAssignmentLabels(labels map[string]string) AssignmentScopeOption {
	return func(scope *AssignmentScope) {
		if len(labels) == 0 {
			return
		}
		if scope.Labels == nil {
			scope.Labels = make(map[string]string, len(labels))
		}
		for key, value := range labels {
			scope.Labels[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
}

// MatchesAccessScope ensures the assignment scope is compatible with the caller scope.
func (s AssignmentScope) MatchesAccessScope(access resource.AccessScope) bool {
	if len(s.GroupIDs) > 0 {
		match := false
		for _, group := range s.GroupIDs {
			if access.MatchesGroup(group) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	if len(s.Labels) > 0 {
		for key, expected := range s.Labels {
			value, ok := access.Label(key)
			if !ok || !strings.EqualFold(value, expected) {
				return false
			}
		}
	}
	return true
}

// HasGroup verifies if the assignment belongs to the specified group.
func (s AssignmentScope) HasGroup(groupID string) bool {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || len(s.GroupIDs) == 0 {
		return false
	}
	for _, candidate := range s.GroupIDs {
		if strings.EqualFold(candidate, groupID) {
			return true
		}
	}
	return false
}

// Assignment permanently binds a principal to a role within an optional tenant/org scope.
type Assignment struct {
	ID           string          `db:"id" json:"id"`
	PrincipalID  string          `db:"principal_id" json:"principalId"`
	RoleName     string          `db:"role_name" json:"roleName"`
	GroupIDsRaw  json.RawMessage `db:"group_ids" json:"-"`
	LabelsRaw    json.RawMessage `db:"labels" json:"-"`
	GroupScope   string          `db:"group_scope" json:"-"`
	LabelScope   string          `db:"label_scope" json:"-"`
	OrgUnitID    *string         `db:"org_unit_id" json:"orgUnitId,omitempty"`
	ResourceType *string         `db:"resource_type" json:"resourceType,omitempty"`
	ResourceID   *string         `db:"resource_id" json:"resourceId,omitempty"`
	CreatedBy    string          `db:"created_by" json:"createdBy"`
	CreatedAt    time.Time       `db:"created_at" json:"createdAt"`
	ExpiresAt    *time.Time      `db:"expires_at" json:"expiresAt,omitempty"`
	Attributes   json.RawMessage `db:"attributes" json:"attributes,omitempty"`
	Scope        AssignmentScope `db:"-" json:"scope,omitempty"`
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

// HydrateScope reconstructs the AssignmentScope from persisted columns.
func (a *Assignment) HydrateScope() {
	if a == nil {
		return
	}
	scope := NewAssignmentScope()
	if len(a.GroupIDsRaw) > 0 {
		var groups []string
		if err := json.Unmarshal(a.GroupIDsRaw, &groups); err == nil {
			scope.GroupIDs = normalizeAssignmentSlice(groups)
		}
	}
	if len(a.LabelsRaw) > 0 {
		var labels map[string]string
		if err := json.Unmarshal(a.LabelsRaw, &labels); err == nil {
			scope.Labels = normalizeAssignmentLabels(labels)
		}
	}
	a.Scope = scope
}

// PrepareScopeColumns normalizes the scope and serializes it to DB columns before persistence.
func (a *Assignment) PrepareScopeColumns() {
	if a == nil {
		return
	}
	a.Scope.GroupIDs = normalizeAssignmentSlice(a.Scope.GroupIDs)
	a.Scope.Labels = normalizeAssignmentLabels(a.Scope.Labels)
	if len(a.Scope.GroupIDs) > 0 {
		if raw, err := json.Marshal(a.Scope.GroupIDs); err == nil {
			a.GroupIDsRaw = raw
		}
		a.GroupScope = hashScopeValues(a.Scope.GroupIDs)
	} else {
		a.GroupIDsRaw = json.RawMessage("[]")
		a.GroupScope = ""
	}
	if len(a.Scope.Labels) > 0 {
		if raw, err := json.Marshal(a.Scope.Labels); err == nil {
			a.LabelsRaw = raw
		}
		a.LabelScope = hashScopeValues(scopeLabelPairs(a.Scope.Labels))
	} else {
		a.LabelsRaw = json.RawMessage("{}")
		a.LabelScope = ""
	}
}

func normalizeAssignmentSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeAssignmentLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		result[trimmedKey] = strings.TrimSpace(value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func scopeLabelPairs(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+strings.TrimSpace(labels[key]))
	}
	return pairs
}

func hashScopeValues(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	h := sha1.New()
	for idx, part := range parts {
		if idx > 0 {
			h.Write([]byte("|"))
		}
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}
