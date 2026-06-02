package lease

import (
	"context"
	"strings"
)

type tenantContextKey struct{}

// WithTenantContext annotates a context with tenant metadata for helper lookups.
func WithTenantContext(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantContextKey{}, strings.TrimSpace(tenantID))
}

// TenantIDFromContext extracts tenant metadata used by lease helper methods.
func TenantIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tenantID, _ := ctx.Value(tenantContextKey{}).(string)
	return strings.TrimSpace(tenantID)
}
