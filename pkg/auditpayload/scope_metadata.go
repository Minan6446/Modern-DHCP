package auditpayload

import "strings"

// ScopeMetadata captures the logical scope details that produced an audit record.
type ScopeMetadata struct {
	ScopeID  string            `json:"scopeId,omitempty"`
	TenantID string            `json:"tenantId,omitempty"`
	GroupIDs []string          `json:"groupIds,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// IsZero reports whether the metadata does not carry any identifying values.
func (m ScopeMetadata) IsZero() bool {
	return strings.TrimSpace(m.ScopeID) == "" &&
		strings.TrimSpace(m.TenantID) == "" &&
		len(m.GroupIDs) == 0 &&
		len(m.Labels) == 0
}

// ScopeFromTenant builds metadata that references only the tenant identifier.
func ScopeFromTenant(tenantID string) ScopeMetadata {
	tenant := strings.TrimSpace(tenantID)
	if tenant == "" {
		return ScopeMetadata{}
	}
	return ScopeMetadata{TenantID: tenant}
}
