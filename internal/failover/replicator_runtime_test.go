package failover

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

type fakeReplicationStreamReader struct {
	messages []replicationStreamMessage
	index    int
	closed   bool
}

func (r *fakeReplicationStreamReader) ReadMessage(ctx context.Context) (replicationStreamMessage, error) {
	if r.index < len(r.messages) {
		msg := r.messages[r.index]
		r.index++
		return msg, nil
	}
	<-ctx.Done()
	return replicationStreamMessage{}, ctx.Err()
}

func (r *fakeReplicationStreamReader) Close() error {
	r.closed = true
	return nil
}

func TestRuntimeReplicatorStandaloneHealthy(t *testing.T) {
	cfg := config.HAConfig{Replication: config.ReplicationConfig{SnapshotInterval: 20 * time.Millisecond}}
	r, ok := buildReplicator(cfg, zap.NewNop()).(*runtimeReplicator)
	if !ok {
		t.Fatalf("expected runtimeReplicator")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	r.Stop()
	tele := r.ReplicationTelemetry()
	if !tele.Healthy {
		t.Fatalf("expected standalone replicator healthy")
	}
	if tele.Mode != "standalone" || tele.Source != "local" || tele.State != "standalone" {
		t.Fatalf("expected standalone telemetry semantics, got %+v", tele)
	}
}

func TestRuntimeReplicatorPartnerProbeFailure(t *testing.T) {
	cfg := config.HAConfig{
		Partner: config.PartnerConfig{Enabled: true, Address: "127.0.0.1", Port: 9},
		Replication: config.ReplicationConfig{
			SnapshotInterval: 20 * time.Millisecond,
			AckTimeout:       20 * time.Millisecond,
		},
	}
	r, ok := buildReplicator(cfg, zap.NewNop()).(*runtimeReplicator)
	if !ok {
		t.Fatalf("expected runtimeReplicator")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	r.Stop()
	tele := r.ReplicationTelemetry()
	if tele.Healthy {
		t.Fatalf("expected unhealthy telemetry when partner probe fails")
	}
	if tele.LastError == "" {
		t.Fatalf("expected probe error message")
	}
	if tele.Mode != "snapshot-probe" || tele.State != "disconnected" {
		t.Fatalf("expected probe semantics on failure, got %+v", tele)
	}
}

func TestRuntimeReplicatorBinlogModeReportsApplyingState(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected tcp addr")
	}
	cfg := config.HAConfig{
		Partner: config.PartnerConfig{Enabled: true, Address: addr.IP.String(), Port: addr.Port},
		Replication: config.ReplicationConfig{
			BinlogSource:     "mysql-primary:3306/mysql-bin.000001",
			SnapshotInterval: 20 * time.Millisecond,
			AckTimeout:       20 * time.Millisecond,
		},
	}
	r, ok := buildReplicator(cfg, zap.NewNop()).(*runtimeReplicator)
	if !ok {
		t.Fatalf("expected runtimeReplicator")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	r.Stop()
	tele := r.ReplicationTelemetry()
	if !tele.Healthy {
		t.Fatalf("expected healthy telemetry, got %+v", tele)
	}
	if tele.Mode != "binlog" || tele.Source != "mysql-primary:3306/mysql-bin.000001" || tele.State != "applying" {
		t.Fatalf("expected binlog applying semantics, got %+v", tele)
	}
	if tele.LastOffset == "" {
		t.Fatalf("expected synthetic replication offset")
	}
}

func TestRuntimeReplicatorCDCModeConsumesRealOffsets(t *testing.T) {
	cfg := config.HAConfig{
		Node: config.HANodeMetadata{ID: "node-b"},
		Replication: config.ReplicationConfig{
			CDCEnabled:       true,
			Brokers:          []string{"kafka-1:9092"},
			Topic:            "lease-journal",
			SnapshotInterval: 20 * time.Millisecond,
		},
	}
	r, ok := buildReplicator(cfg, zap.NewNop()).(*runtimeReplicator)
	if !ok {
		t.Fatalf("expected runtimeReplicator")
	}
	r.readerFactory = func(cfg config.HAConfig, logger *zap.Logger) (replicationStreamReader, error) {
		return &fakeReplicationStreamReader{messages: []replicationStreamMessage{{
			Partition: 2,
			Offset:    41,
			Time:      time.Now().UTC().Add(-250 * time.Millisecond),
			Value:     []byte(`{"leaseId":"lease-1","dedupeKey":"tenant-a|pool-a|10.0.0.10|active|lease-1|2026-04-14T00:00:00Z","occurredAt":"2026-04-14T00:00:00Z"}`),
		}}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	time.Sleep(40 * time.Millisecond)
	cancel()
	r.Stop()
	tele := r.ReplicationTelemetry()
	if !tele.Healthy {
		t.Fatalf("expected healthy telemetry, got %+v", tele)
	}
	if tele.Mode != "cdc" || tele.Source != "kafka-1:9092/lease-journal" {
		t.Fatalf("expected cdc stream source, got %+v", tele)
	}
	if tele.State != "catching_up" && tele.State != "applying" {
		t.Fatalf("expected stream state, got %+v", tele)
	}
	if tele.LastOffset == "" || !strings.Contains(tele.LastOffset, "lease-journal") || !strings.Contains(tele.LastOffset, "o41") {
		t.Fatalf("expected real stream offset, got %+v", tele)
	}
	if tele.ApplyLagMs < 0 {
		t.Fatalf("expected non-negative lag, got %+v", tele)
	}
}
