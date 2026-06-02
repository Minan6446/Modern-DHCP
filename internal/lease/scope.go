package lease

import (
	"strings"

	"modern-dhcp/internal/resource"
)

var ErrResourceScopeRequired = resource.ErrScopeRequired

type ResourceScope struct {
	access resource.AccessScope
}

const defaultLeaseScopePrincipal = "lease.scope.bridge"

// ResourceScopeFromAccess preserves the caller's access metadata for downstream services.
func ResourceScopeFromAccess(access resource.AccessScope) ResourceScope {
	return ResourceScope{access: ensureLeasePrincipal(access)}
}

// NewResourceScope constructs a legacy-compatible scope from explicit identifiers.
func NewResourceScope(scopeID, tenantID string) ResourceScope {
	opts := make([]resource.AccessScopeOption, 0, 1)
	if trimmed := strings.TrimSpace(tenantID); trimmed != "" {
		opts = append(opts, resource.WithTenantID(trimmed))
	}
	access := resource.NewAccessScope(scopeID, defaultLeaseScopePrincipal, opts...)
	return ResourceScope{access: access}
}

// AccessScope exposes the richer scope payload for repositories.
func (s ResourceScope) AccessScope() resource.AccessScope {
	return ensureLeasePrincipal(s.access)
}

// TenantOrDefault returns the tenant identifier or falls back to the logical scope.
func (s ResourceScope) TenantOrDefault() string {
	return strings.TrimSpace(s.AccessScope().EffectiveTenant())
}

// TenantIDOrErr enforces that a tenant identifier is present.
func (s ResourceScope) TenantIDOrErr() (string, error) {
	tenant := s.TenantOrDefault()
	if tenant == "" {
		return "", ErrResourceScopeRequired
	}
	return tenant, nil
}

// WithTenantOverride preserves the existing access metadata while forcing a specific tenant identifier.
func (s ResourceScope) WithTenantOverride(tenantID string) ResourceScope {
	if strings.TrimSpace(tenantID) == "" {
		return s
	}
	access := s.AccessScope()
	resource.WithTenantID(tenantID)(&access)
	return ResourceScope{access: access}
}

// ScopeOrDefault returns the logical scope identifier.
func (s ResourceScope) ScopeOrDefault() string {
	scope := strings.TrimSpace(s.AccessScope().ScopeID)
	if scope != "" {
		return scope
	}
	return s.TenantOrDefault()
}

// IsZero reports whether the scope contains any identifying metadata.
func (s ResourceScope) IsZero() bool {
	scope := strings.TrimSpace(s.AccessScope().ScopeID)
	tenant := s.TenantOrDefault()
	return scope == "" && tenant == ""
}

// LegacyScope returns a compatibility view for components that still expect resource.Scope.
func (s ResourceScope) LegacyScope() resource.Scope {
	return s.AccessScope().LegacyScope()
}

func ensureLeasePrincipal(scope resource.AccessScope) resource.AccessScope {
	if strings.TrimSpace(scope.Principal) == "" {
		scope.Principal = defaultLeaseScopePrincipal
	}
	return scope
}
