package failover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

type replicationStreamMessage struct {
	Partition int
	Offset    int64
	Time      time.Time
	Key       []byte
	Value     []byte
}

type replicationStreamReader interface {
	ReadMessage(ctx context.Context) (replicationStreamMessage, error)
	Close() error
}

type replicationStreamReaderFactory func(cfg config.HAConfig, logger *zap.Logger) (replicationStreamReader, error)

type kafkaReplicationReader struct {
	reader *kafka.Reader
}

func (r *kafkaReplicationReader) ReadMessage(ctx context.Context) (replicationStreamMessage, error) {
	if r == nil || r.reader == nil {
		return replicationStreamMessage{}, fmt.Errorf("replication stream reader unavailable")
	}
	msg, err := r.reader.ReadMessage(ctx)
	if err != nil {
		return replicationStreamMessage{}, err
	}
	return replicationStreamMessage{
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Time:      msg.Time,
		Key:       append([]byte(nil), msg.Key...),
		Value:     append([]byte(nil), msg.Value...),
	}, nil
}

func (r *kafkaReplicationReader) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

type replicationJournalEnvelope struct {
	OccurredAt time.Time `json:"occurredAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LeaseID    string    `json:"leaseId"`
	TenantID   string    `json:"tenantId"`
	PoolID     string    `json:"poolId"`
	IPAddress  string    `json:"ip"`
	DedupeKey  string    `json:"dedupeKey"`
}

type runtimeReplicator struct {
	cfg           config.HAConfig
	logger        *zap.Logger
	mu            sync.RWMutex
	healthy       bool
	telemetry     ReplicationTelemetry
	cancel        context.CancelFunc
	done          chan struct{}
	sequence      uint64
	readerFactory replicationStreamReaderFactory
}

func buildReplicator(cfg config.HAConfig, logger *zap.Logger) Replicator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &runtimeReplicator{
		cfg:           cfg,
		logger:        logger,
		healthy:       true,
		telemetry:     ReplicationTelemetry{Healthy: true, UpdatedAt: time.Now().UTC(), Mode: replicationMode(cfg), Source: replicationSource(cfg), State: initialReplicationState(cfg)},
		readerFactory: newReplicationStreamReader,
	}
}

func (r *runtimeReplicator) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.cancel = cancel
	r.done = done
	r.mu.Unlock()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			r.Stop()
		}()
	}
	go r.loop(loopCtx, done)
}

func (r *runtimeReplicator) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (r *runtimeReplicator) Healthy() bool {
	if r == nil {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.healthy
}

func (r *runtimeReplicator) ReplicationTelemetry() ReplicationTelemetry {
	if r == nil {
		return ReplicationTelemetry{Healthy: true}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.telemetry
}

func (r *runtimeReplicator) loop(ctx context.Context, done chan struct{}) {
	defer func() {
		if done != nil {
			close(done)
		}
	}()

	if r.streamCapable() {
		r.consumeStream(ctx)
		return
	}

	interval := r.cfg.Replication.SnapshotInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.probeOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.probeOnce()
		}
	}
}

func (r *runtimeReplicator) consumeStream(ctx context.Context) {
	mode := replicationMode(r.cfg)
	source := replicationSource(r.cfg)
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		reader, err := r.openStreamReader()
		if err != nil {
			r.storeTelemetry(ReplicationTelemetry{
				Healthy:   false,
				UpdatedAt: time.Now().UTC(),
				LastError: err.Error(),
				Mode:      mode,
				Source:    source,
				State:     "error",
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		r.storeTelemetry(ReplicationTelemetry{
			Healthy:   true,
			UpdatedAt: time.Now().UTC(),
			Mode:      mode,
			Source:    source,
			State:     "catching_up",
		})
		for {
			message, readErr := reader.ReadMessage(ctx)
			if readErr != nil {
				_ = reader.Close()
				if ctx.Err() != nil || errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
					return
				}
				r.storeTelemetry(ReplicationTelemetry{
					Healthy:   false,
					UpdatedAt: time.Now().UTC(),
					LastError: readErr.Error(),
					Mode:      mode,
					Source:    source,
					State:     "error",
				})
				break
			}
			r.storeTelemetry(replicationTelemetryFromMessage(r.cfg, message))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(350 * time.Millisecond):
		}
	}
}

func (r *runtimeReplicator) probeOnce() {
	now := time.Now().UTC()
	addr := strings.TrimSpace(r.cfg.Partner.Address)
	mode := replicationMode(r.cfg)
	source := replicationSource(r.cfg)
	port := r.cfg.Partner.Port
	if port <= 0 {
		port = 647
	}
	timeout := r.cfg.Replication.AckTimeout
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	if !r.cfg.Partner.Enabled || addr == "" {
		r.storeTelemetry(ReplicationTelemetry{
			Healthy:    true,
			UpdatedAt:  now,
			LastError:  "",
			LastOffset: "standalone",
			Mode:       mode,
			Source:     source,
			State:      "standalone",
		})
		return
	}
	target := net.JoinHostPort(addr, fmt.Sprintf("%d", port))
	if source == "" {
		source = target
	}
	conn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		previous := r.ReplicationTelemetry()
		r.storeTelemetry(ReplicationTelemetry{
			Healthy:    false,
			UpdatedAt:  now,
			ApplyLagMs: replicationStallLag(previous, now),
			LastError:  err.Error(),
			LastOffset: retainedOffset(previous.LastOffset, target),
			Mode:       mode,
			Source:     source,
			State:      "disconnected",
		})
		if r.logger != nil {
			r.logger.Debug("replicator probe failed", zap.String("target", target), zap.Error(err))
		}
		return
	}
	_ = conn.Close()
	sequence := atomic.AddUint64(&r.sequence, 1)
	r.storeTelemetry(ReplicationTelemetry{
		Healthy:    true,
		UpdatedAt:  now,
		ApplyLagMs: 0,
		LastError:  "",
		LastOffset: replicationOffset(mode, source, sequence),
		Mode:       mode,
		Source:     source,
		State:      healthyReplicationState(mode),
	})
}

func (r *runtimeReplicator) storeTelemetry(telemetry ReplicationTelemetry) {
	r.mu.Lock()
	r.telemetry = telemetry
	r.healthy = telemetry.Healthy
	r.mu.Unlock()
}

func (r *runtimeReplicator) streamCapable() bool {
	if r == nil || r.readerFactory == nil {
		return false
	}
	mode := replicationMode(r.cfg)
	if mode != "cdc" && mode != "event-stream" {
		return false
	}
	return len(r.cfg.Replication.Brokers) > 0 && strings.TrimSpace(r.cfg.Replication.Topic) != ""
}

func (r *runtimeReplicator) openStreamReader() (replicationStreamReader, error) {
	if r == nil || r.readerFactory == nil {
		return nil, fmt.Errorf("replication stream factory unavailable")
	}
	return r.readerFactory(r.cfg, r.logger)
}

func newReplicationStreamReader(cfg config.HAConfig, logger *zap.Logger) (replicationStreamReader, error) {
	if len(cfg.Replication.Brokers) == 0 || strings.TrimSpace(cfg.Replication.Topic) == "" {
		return nil, fmt.Errorf("replication stream requires brokers and topic")
	}
	groupID := strings.TrimSpace(cfg.Node.ID)
	if groupID == "" {
		groupID = "default"
	}
	groupID = "modern-dhcp-replicator-" + strings.ToLower(groupID)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        append([]string(nil), cfg.Replication.Brokers...),
		Topic:          strings.TrimSpace(cfg.Replication.Topic),
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        500 * time.Millisecond,
		CommitInterval: 250 * time.Millisecond,
	})
	if logger != nil {
		logger.Info("replication stream reader enabled",
			zap.String("topic", strings.TrimSpace(cfg.Replication.Topic)),
			zap.String("groupId", groupID),
			zap.Strings("brokers", append([]string(nil), cfg.Replication.Brokers...)),
		)
	}
	return &kafkaReplicationReader{reader: reader}, nil
}

func replicationTelemetryFromMessage(cfg config.HAConfig, message replicationStreamMessage) ReplicationTelemetry {
	now := time.Now().UTC()
	telemetry := ReplicationTelemetry{
		Healthy:    true,
		UpdatedAt:  now,
		LastOffset: fmt.Sprintf("%s:p%d:o%d", strings.TrimSpace(cfg.Replication.Topic), message.Partition, message.Offset),
		Mode:       replicationMode(cfg),
		Source:     replicationSource(cfg),
		State:      "applying",
	}
	envelope := replicationJournalEnvelope{}
	if len(message.Value) > 0 && json.Unmarshal(message.Value, &envelope) == nil {
		telemetry.LastOffset = replicationOffsetFromEnvelope(cfg, message, envelope)
		if occurredAt := coalesceJournalTime(envelope.OccurredAt, envelope.UpdatedAt, message.Time); !occurredAt.IsZero() {
			lag := now.Sub(occurredAt.UTC()).Milliseconds()
			if lag > 0 {
				telemetry.ApplyLagMs = int(lag)
			}
			if telemetry.ApplyLagMs > 0 {
				telemetry.State = "catching_up"
			}
		}
	}
	if telemetry.ApplyLagMs <= 0 && !message.Time.IsZero() {
		lag := now.Sub(message.Time.UTC()).Milliseconds()
		if lag > 0 {
			telemetry.ApplyLagMs = int(lag)
		}
	}
	return telemetry
}

func replicationOffsetFromEnvelope(cfg config.HAConfig, message replicationStreamMessage, envelope replicationJournalEnvelope) string {
	source := strings.TrimSpace(replicationSource(cfg))
	if dedupe := strings.TrimSpace(envelope.DedupeKey); dedupe != "" {
		return fmt.Sprintf("%s|p%d|o%d|%s", source, message.Partition, message.Offset, dedupe)
	}
	if leaseID := strings.TrimSpace(envelope.LeaseID); leaseID != "" {
		return fmt.Sprintf("%s|p%d|o%d|lease=%s", source, message.Partition, message.Offset, leaseID)
	}
	return fmt.Sprintf("%s|p%d|o%d", source, message.Partition, message.Offset)
}

func coalesceJournalTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}

func replicationMode(cfg config.HAConfig) string {
	if strings.TrimSpace(cfg.Replication.BinlogSource) != "" {
		return "binlog"
	}
	if cfg.Replication.CDCEnabled {
		return "cdc"
	}
	if strings.TrimSpace(cfg.Replication.Topic) != "" || len(cfg.Replication.Brokers) > 0 {
		return "event-stream"
	}
	if !cfg.Partner.Enabled || strings.TrimSpace(cfg.Partner.Address) == "" {
		return "standalone"
	}
	return "snapshot-probe"
}

func replicationSource(cfg config.HAConfig) string {
	if source := strings.TrimSpace(cfg.Replication.BinlogSource); source != "" {
		return source
	}
	if topic := strings.TrimSpace(cfg.Replication.Topic); topic != "" {
		if len(cfg.Replication.Brokers) > 0 {
			return strings.TrimSpace(cfg.Replication.Brokers[0]) + "/" + topic
		}
		return topic
	}
	if len(cfg.Replication.Brokers) > 0 {
		return strings.TrimSpace(cfg.Replication.Brokers[0])
	}
	if !cfg.Partner.Enabled || strings.TrimSpace(cfg.Partner.Address) == "" {
		return "local"
	}
	port := cfg.Partner.Port
	if port <= 0 {
		port = 647
	}
	return net.JoinHostPort(strings.TrimSpace(cfg.Partner.Address), fmt.Sprintf("%d", port))
}

func initialReplicationState(cfg config.HAConfig) string {
	if replicationMode(cfg) == "standalone" {
		return "standalone"
	}
	return "starting"
}

func healthyReplicationState(mode string) string {
	switch strings.TrimSpace(mode) {
	case "binlog", "cdc", "event-stream":
		return "applying"
	case "standalone":
		return "standalone"
	default:
		return "connected"
	}
}

func replicationOffset(mode, source string, sequence uint64) string {
	mode = strings.TrimSpace(mode)
	source = strings.TrimSpace(source)
	if mode == "" {
		mode = "snapshot-probe"
	}
	if source == "" {
		source = "peer"
	}
	return fmt.Sprintf("%s:%s#%06d", mode, source, sequence)
}

func replicationStallLag(previous ReplicationTelemetry, now time.Time) int {
	if previous.UpdatedAt.IsZero() {
		return 0
	}
	lag := now.Sub(previous.UpdatedAt.UTC()).Milliseconds()
	if lag < 0 {
		return 0
	}
	return int(lag)
}

func retainedOffset(offset, fallback string) string {
	if value := strings.TrimSpace(offset); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}
