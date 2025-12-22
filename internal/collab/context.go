package collab

import "context"

type contextKey string

const bootstrapKey contextKey = "collabBootstrap"

// SessionBootstrap carries the scope we expect a WebSocket client to operate within.
type SessionBootstrap struct {
	TenantID     string
	UserID       string
	ResourceType string
	ResourceID   string
	Attributes   map[string]string
}

// ContextWithBootstrap annotates a context with the bootstrap payload for downstream lookup.
func ContextWithBootstrap(ctx context.Context, bootstrap SessionBootstrap) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, bootstrapKey, bootstrap)
}

// BootstrapFromContext extracts the bootstrap payload when present.
func BootstrapFromContext(ctx context.Context) (SessionBootstrap, bool) {
	if ctx == nil {
		return SessionBootstrap{}, false
	}
	value, ok := ctx.Value(bootstrapKey).(SessionBootstrap)
	if !ok {
		return SessionBootstrap{}, false
	}
	if !value.Valid() {
		return SessionBootstrap{}, false
	}
	return value, true
}

// Valid returns true when the bootstrap payload contains the required context.
func (s SessionBootstrap) Valid() bool {
	return s.TenantID != "" && s.UserID != "" && s.ResourceType != "" && s.ResourceID != ""
}
