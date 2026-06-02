package resource

import (
	"errors"
	"sort"
	"strings"
)

var ErrScopeRequired = errors.New("resource scope required")

type Scope struct {
	ID       string
	TenantID string
}

// AccessScope represents the caller + grouping context that higher-level services should honor.
type AccessScope struct {
	ScopeID   string            `json:"scopeId"`
	TenantID  *string           `json:"tenantId,omitempty"`
	GroupIDs  []string          `json:"groupIds,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Principal string            `json:"principal"`
}

// AccessScopeOption mutates an AccessScope during construction.
type AccessScopeOption func(*AccessScope)

// NewAccessScope builds an AccessScope with functional options to express optional fields.
func NewAccessScope(scopeID, principal string, opts ...AccessScopeOption) AccessScope {
	scope := AccessScope{ScopeID: strings.TrimSpace(scopeID), Principal: strings.TrimSpace(principal)}
	for _, opt := range opts {
		if opt != nil {
			opt(&scope)
		}
	}
	scope.GroupIDs = normalizeStringSlice(scope.GroupIDs)
	if len(scope.Labels) == 0 {
		scope.Labels = nil
	} else {
		scope.Labels = normalizeLabelMap(scope.Labels)
	}
	scope.TenantID = normalizeOptional(scope.TenantID)
	return scope
}

// WithTenantID sets an optional tenant identifier.
func WithTenantID(tenantID string) AccessScopeOption {
	return func(scope *AccessScope) {
		scope.TenantID = normalizeOptional(ptrString(tenantID))
	}
}

// WithGroupIDs records one or more grouping identifiers (departments, projects, etc.).
func WithGroupIDs(groups ...string) AccessScopeOption {
	return func(scope *AccessScope) {
		scope.GroupIDs = append(scope.GroupIDs, groups...)
	}
}

// WithLabels copies arbitrary isolation labels (e.g., region, environment) onto the scope.
func WithLabels(labels map[string]string) AccessScopeOption {
	return func(scope *AccessScope) {
		if len(labels) == 0 {
			return
		}
		if scope.Labels == nil {
			scope.Labels = make(map[string]string, len(labels))
		}
		for k, v := range labels {
			scope.Labels[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
}

// WithLabel appends a single label entry.
func WithLabel(key, value string) AccessScopeOption {
	return func(scope *AccessScope) {
		if scope.Labels == nil {
			scope.Labels = make(map[string]string)
		}
		scope.Labels[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

// EffectiveTenant returns the tenant identifier after applying fallbacks.
func (s AccessScope) EffectiveTenant() string {
	if s.TenantID != nil {
		return strings.TrimSpace(*s.TenantID)
	}
	return strings.TrimSpace(s.ScopeID)
}

// HasLabel reports whether the label with key matches the provided value.
func (s AccessScope) HasLabel(key, value string) bool {
	actual, ok := s.Label(key)
	return ok && actual == strings.TrimSpace(value)
}

// Label fetches a label value paired with the key.
func (s AccessScope) Label(key string) (string, bool) {
	if len(s.Labels) == 0 {
		return "", false
	}
	value, ok := s.Labels[strings.TrimSpace(key)]
	return value, ok
}

// MatchesGroup reports whether the scope contains the provided group identifier.
func (s AccessScope) MatchesGroup(groupID string) bool {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return len(s.GroupIDs) == 0
	}
	for _, candidate := range s.GroupIDs {
		if strings.EqualFold(candidate, groupID) {
			return true
		}
	}
	return false
}

// LegacyScope converts the richer AccessScope into the existing two-field Scope for backward compatibility.
func (s AccessScope) LegacyScope() Scope {
	tenant := ""
	if s.TenantID != nil {
		tenant = *s.TenantID
	}
	return Scope{ID: strings.TrimSpace(s.ScopeID), TenantID: strings.TrimSpace(tenant)}
}

func NewScope(scopeID, tenantID string) Scope {
	return Scope{ID: strings.TrimSpace(scopeID), TenantID: strings.TrimSpace(tenantID)}
}

func (s Scope) TenantOrDefault() string {
	if tenant := strings.TrimSpace(s.TenantID); tenant != "" {
		return tenant
	}
	return strings.TrimSpace(s.ID)
}

func (s Scope) ScopeOrDefault() string {
	if scope := strings.TrimSpace(s.ID); scope != "" {
		return scope
	}
	return strings.TrimSpace(s.TenantID)
}

func (s Scope) IsZero() bool {
	return strings.TrimSpace(s.ID) == "" && strings.TrimSpace(s.TenantID) == ""
}

func (s Scope) TenantIDOrErr() (string, error) {
	tenant := s.TenantOrDefault()
	if tenant == "" {
		return "", ErrScopeRequired
	}
	return tenant, nil
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return ptrString(trimmed)
}

func normalizeStringSlice(values []string) []string {
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

func normalizeLabelMap(labels map[string]string) map[string]string {
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

func ptrString(value string) *string {
	copy := strings.TrimSpace(value)
	if copy == "" {
		return nil
	}
	return &copy
}
