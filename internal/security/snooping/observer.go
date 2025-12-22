package snooping

import "time"

// ObservationResult categorizes snooping lookup outcomes.
type ObservationResult string

const (
	ObservationResultTrusted   ObservationResult = "trusted"
	ObservationResultMiss      ObservationResult = "miss"
	ObservationResultUntrusted ObservationResult = "untrusted"
	ObservationResultError     ObservationResult = "error"
)

// Observation captures metadata for a snooping lookup verdict.
type Observation struct {
	TenantID   string
	MAC        string
	PortID     string
	VLANID     int
	Result     ObservationResult
	Reason     string
	OccurredAt time.Time
}

// Observer records snooping activity for downstream consumers.
type Observer interface {
	Record(obs Observation)
}
