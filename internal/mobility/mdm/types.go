package mdm

import (
	"context"
	"time"
)

// ComplianceRecord represents the outcome of an MDM compliance lookup.
type ComplianceRecord struct {
	TenantID   string
	DeviceID   string
	Managed    bool
	Tags       []string
	Source     string
	ObservedAt time.Time
	ExpiresAt  time.Time
}

// Connector represents a concrete MDM integration capable of producing compliance records.
type Connector interface {
	Name() string
	Enabled() bool
	Sync(ctx context.Context) ([]ComplianceRecord, error)
}
