package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
	"modern-dhcp/pkg/models"
)

type leaseSyncWireMessage struct {
	LeaseID    string `json:"leaseId"`
	TenantID   string `json:"tenantId"`
	PoolID     string `json:"poolId"`
	IPAddress  string `json:"ipAddress"`
	State      string `json:"state"`
	Phase      string `json:"phase"`
	OccurredAt string `json:"occurredAt"`
}

type leaseSyncWireAck struct {
	LeaseID string `json:"leaseId"`
	Phase   string `json:"phase,omitempty"`
	Ack     bool   `json:"ack"`
	Error   string `json:"error,omitempty"`
	AckAt   string `json:"ackAt"`
}

// FailoverAckGate confirms lease sync on HA partner before response paths continue.
type FailoverAckGate struct {
	address string
	timeout time.Duration
	logger  *zap.Logger
}

func NewFailoverAckGate(cfg config.HAConfig, logger *zap.Logger) *FailoverAckGate {
	if !cfg.Partner.Enabled {
		return nil
	}
	host := strings.TrimSpace(cfg.Partner.Address)
	if host == "" {
		return nil
	}
	port := cfg.Partner.Port
	if port <= 0 {
		port = 647
	}
	timeout := cfg.Replication.AckTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FailoverAckGate{address: net.JoinHostPort(host, fmt.Sprintf("%d", port)), timeout: timeout, logger: logger}
}

func (g *FailoverAckGate) AwaitLeaseSyncAck(ctx context.Context, lease *models.Lease) error {
	if g == nil || lease == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	dialer := &net.Dialer{Timeout: g.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", g.address)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(g.timeout))

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	prepare := leaseSyncWireMessage{
		LeaseID:    lease.ID,
		TenantID:   lease.TenantID,
		PoolID:     lease.PoolID,
		IPAddress:  lease.IPAddress,
		State:      lease.State,
		Phase:      "prepare",
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := enc.Encode(prepare); err != nil {
		return err
	}
	var prepareAck leaseSyncWireAck
	if err := dec.Decode(&prepareAck); err != nil {
		return err
	}
	if !prepareAck.Ack {
		if strings.TrimSpace(prepareAck.Error) != "" {
			return fmt.Errorf(prepareAck.Error)
		}
		return fmt.Errorf("partner rejected prepare")
	}

	commit := prepare
	commit.Phase = "commit"
	commit.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := enc.Encode(commit); err != nil {
		return err
	}
	var commitAck leaseSyncWireAck
	if err := dec.Decode(&commitAck); err != nil {
		return err
	}
	if !commitAck.Ack {
		if strings.TrimSpace(commitAck.Error) != "" {
			return fmt.Errorf(commitAck.Error)
		}
		return fmt.Errorf("partner rejected commit")
	}
	return nil
}
