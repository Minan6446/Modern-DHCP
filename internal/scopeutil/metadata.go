package scopeutil

import (
	"strings"

	"modern-dhcp/internal/lease"
	"modern-dhcp/pkg/auditpayload"
)

// FromLeaseScope converts a lease.ResourceScope into reusable metadata for logs and APIs.
func FromLeaseScope(scope lease.ResourceScope) auditpayload.ScopeMetadata {
	access := scope.AccessScope()
	meta := auditpayload.ScopeFromTenant(scope.TenantOrDefault())
	meta.ScopeID = strings.TrimSpace(access.ScopeID)
	if access.TenantID != nil {
		meta.TenantID = strings.TrimSpace(*access.TenantID)
	}
	if len(access.GroupIDs) > 0 {
		meta.GroupIDs = append(meta.GroupIDs, access.GroupIDs...)
	}
	if len(access.Labels) > 0 {
		labels := make(map[string]string, len(access.Labels))
		for key, value := range access.Labels {
			labels[key] = value
		}
		meta.Labels = labels
	}
	if strings.TrimSpace(meta.TenantID) == "" {
		if tenant := strings.TrimSpace(scope.TenantOrDefault()); tenant != "" && tenant != meta.ScopeID {
			meta.TenantID = tenant
		}
	}
	if meta.IsZero() {
		return auditpayload.ScopeMetadata{}
	}
	return meta
}
