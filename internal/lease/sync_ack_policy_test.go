package lease

import (
	"context"
	"errors"
	"testing"

	"modern-dhcp/pkg/models"
)

type stubSyncAckGate struct {
	err error
}

func (s stubSyncAckGate) AwaitLeaseSyncAck(ctx context.Context, lease *models.Lease) error {
	return s.err
}

func TestConfirmSyncAckStrict(t *testing.T) {
	svc := &Service{syncAckGate: stubSyncAckGate{err: errors.New("ack failed")}, syncAckPolicy: "strict"}
	err := svc.confirmSyncAck(context.Background(), &models.Lease{ID: "l-1"})
	if err == nil {
		t.Fatal("expected strict mode to return error")
	}
}

func TestConfirmSyncAckDegraded(t *testing.T) {
	svc := &Service{syncAckGate: stubSyncAckGate{err: errors.New("ack failed")}, syncAckPolicy: "degraded"}
	if err := svc.confirmSyncAck(context.Background(), &models.Lease{ID: "l-1"}); err != nil {
		t.Fatalf("expected degraded mode to swallow error, got %v", err)
	}
}
