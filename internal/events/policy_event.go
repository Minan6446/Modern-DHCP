package events

import (
	"context"
	"encoding/json"
	"time"
)

// PolicyEvent describes a rule change notification payload.
type PolicyEvent struct {
	Action   string          `json:"action"`
	TenantID string          `json:"tenantId"`
	RuleID   string          `json:"ruleId"`
	Body     json.RawMessage `json:"body,omitempty"`
	Version  int64           `json:"version"`
	At       time.Time       `json:"timestamp"`
}

// PolicyPublisher pushes policy change notifications to downstream systems.
type PolicyPublisher interface {
	Publish(ctx context.Context, evt PolicyEvent) error
	Close(ctx context.Context) error
}
