package ipsgdai

import (
	"context"
	"time"
)

// LeaseSnapshot represents the payload sent to IP-SG/DAI controllers.
type LeaseSnapshot struct {
	TenantID    string
	MACAddress  string
	IPAddress   string
	PoolID      string
	VLANID      *int
	InterfaceID string
	PortID      string
	ExpiresAt   time.Time
	Action      string
	MDMManaged  bool
	MDMSource   string
	MDMTags     []string
	MDMObserved *time.Time
}

// Alert represents feedback from DAI controllers.
type Alert struct {
	TenantID string
	MAC      string
	IP       string
	Reason   string
	Action   string
}

// Publisher pushes lease snapshots to downstream controllers.
type Publisher interface {
	Publish(ctx context.Context, snapshot LeaseSnapshot) error
}

// Callback handles alerts sourced from IP-SG/DAI systems.
type Callback interface {
	HandleAlert(ctx context.Context, alert Alert) error
}

type noopPublisher struct{}

// NewNoopPublisher creates a publisher that discards updates.
func NewNoopPublisher() Publisher {
	return &noopPublisher{}
}

func (n *noopPublisher) Publish(ctx context.Context, snapshot LeaseSnapshot) error {
	return nil
}
