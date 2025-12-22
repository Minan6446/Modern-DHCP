package detector

import (
	"context"
	"time"
)

// Event captures features about a DHCP transaction for behavioral analysis.
type Event struct {
	TenantID    string
	MAC         string
	MessageType string
	CircuitID   string
	RemoteID    string
	PortID      string
	GIAddr      string
	Timestamp   time.Time
}

// Verdict communicates the detector output.
type Verdict struct {
	Block  bool
	Score  float64
	Reason string
	State  SecurityState
}

// SecurityState tracks the standing of a client from the detector's perspective.
type SecurityState string

const (
	SecurityStateOK      SecurityState = "ok"
	SecurityStateSuspect SecurityState = "suspect"
	SecurityStateBlocked SecurityState = "blocked"
)

// Detector consumes events and emits verdicts.
type Detector interface {
	Observe(ctx context.Context, evt Event) Verdict
}

type noopDetector struct{}

// NewNoop returns a detector that never blocks traffic.
func NewNoop() Detector {
	return &noopDetector{}
}

func (n *noopDetector) Observe(ctx context.Context, evt Event) Verdict {
	return Verdict{Block: false, Score: 0}
}
