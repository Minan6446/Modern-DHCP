package notifications

import "time"

// Message represents a normalized notification payload for downstream channels.
type Message struct {
	ID        string            `json:"id"`
	TenantID  string            `json:"tenantId"`
	Topic     string            `json:"topic"`
	Summary   string            `json:"summary"`
	Severity  string            `json:"severity"`
	Body      map[string]any    `json:"body,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Source    string            `json:"source,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}
