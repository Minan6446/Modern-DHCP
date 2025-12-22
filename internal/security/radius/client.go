package radius

import (
	"context"
	"time"
)

// LookupRequest identifies a subscriber or session.
type LookupRequest struct {
	TenantID string
	MAC      string
	ClientID string
}

// Attributes reflect Access-Accept metadata relevant to policy decisions.
type Attributes struct {
	VLANID     *int
	ACL        string
	SessionID  string
	ReplyAttrs map[string]string
}

// AccountingReport summarizes lease lifecycle changes for RADIUS accounting.
type AccountingReport struct {
	TenantID   string
	MACAddress string
	ClientID   string
	IPAddress  string
	Action     string
	ExpiresAt  time.Time
}

// Client issues RADIUS queries and accounting packets.
type Client interface {
	Lookup(ctx context.Context, req LookupRequest) (*Attributes, error)
	Accounting(ctx context.Context, report AccountingReport) error
}

// CoAServer listens for Change of Authorization or Disconnect messages.
type CoAServer interface {
	Listen(ctx context.Context) error
}

type noopClient struct{}

// NewNoopClient returns a client that performs no network I/O.
func NewNoopClient() Client {
	return &noopClient{}
}

func (n *noopClient) Lookup(ctx context.Context, req LookupRequest) (*Attributes, error) {
	return nil, nil
}

func (n *noopClient) Accounting(ctx context.Context, report AccountingReport) error {
	return nil
}
