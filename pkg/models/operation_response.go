package models

import "time"

// OperationStep describes a single human-readable phase in a lifecycle action.
type OperationStep struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status,omitempty"`
}

// ValidationReport captures post-operation verification details.
type ValidationReport struct {
	Status    string    `json:"status"`
	Messages  []string  `json:"messages,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// SyncReport surfaces cluster/failover replication state to operators.
type SyncReport struct {
	Role           string    `json:"role,omitempty"`
	State          string    `json:"state,omitempty"`
	LagSeconds     float64   `json:"lagSeconds,omitempty"`
	PeerLastSeen   time.Time `json:"peerLastSeen,omitempty"`
	PeerHealthyAt  time.Time `json:"peerHealthySince,omitempty"`
	ManualFailback bool      `json:"manualFailback"`
}

// OperationEvent records downstream tasks queued as part of an action.
type OperationEvent struct {
	Channel       string    `json:"channel"`
	Status        string    `json:"status"`
	Detail        string    `json:"detail,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
	EmittedAt     time.Time `json:"emittedAt"`
}

// CleanupReport outlines follow-up work (audit writes, cache eviction, etc.).
type CleanupReport struct {
	Status      string    `json:"status"`
	Tasks       []string  `json:"tasks,omitempty"`
	CompletedAt time.Time `json:"completedAt"`
}

// OperationResponse wraps every mutating API response with telemetry envelopes.
type OperationResponse struct {
	Status     string            `json:"status"`
	Data       any               `json:"data,omitempty"`
	Steps      []OperationStep   `json:"steps,omitempty"`
	Validation *ValidationReport `json:"validation,omitempty"`
	Stats      map[string]any    `json:"stats,omitempty"`
	Sync       *SyncReport       `json:"sync,omitempty"`
	Events     []OperationEvent  `json:"events,omitempty"`
	Cleanup    *CleanupReport    `json:"cleanup,omitempty"`
	TraceID    string            `json:"traceId,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}
