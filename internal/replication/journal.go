package replication

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"modern-dhcp/internal/config"
	"modern-dhcp/pkg/models"
)

// KafkaJournal implements a dual-write CDC confirmation path backed by Kafka.
type KafkaJournal struct {
	logger  *zap.Logger
	writer  *kafka.Writer
	timeout time.Duration
}

// NewKafkaJournal builds a Kafka-backed replication journal if CDC is enabled.
func NewKafkaJournal(cfg config.ReplicationConfig, logger *zap.Logger) *KafkaJournal {
	if !cfg.CDCEnabled {
		return nil
	}
	if len(cfg.Brokers) == 0 || cfg.Topic == "" {
		if logger != nil {
			logger.Warn("replication journal requires brokers and topic; disabling CDC confirmation")
		}
		return nil
	}
	timeout := cfg.AckTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		RequiredAcks: kafka.RequireAll,
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 5 * time.Millisecond,
		WriteTimeout: timeout,
		ReadTimeout:  timeout,
	}
	return &KafkaJournal{logger: logger, writer: writer, timeout: timeout}
}

// ConfirmLease blocks until the journal receives an ACK for the lease payload.
func (k *KafkaJournal) ConfirmLease(ctx context.Context, lease *models.Lease) error {
	if k == nil || k.writer == nil || lease == nil {
		return nil
	}
	payload := journalEnvelope{
		LeaseID:   lease.ID,
		TenantID:  lease.TenantID,
		PoolID:    lease.PoolID,
		IPAddress: lease.IPAddress,
		State:     lease.State,
		UpdatedAt: lease.UpdatedAt,
		Lease:     lease,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()
	return k.writer.WriteMessages(cctx, kafka.Message{
		Key:   []byte(lease.ID),
		Value: data,
	})
}

// Close shuts down the underlying writer.
func (k *KafkaJournal) Close() error {
	if k == nil || k.writer == nil {
		return nil
	}
	return k.writer.Close()
}

type journalEnvelope struct {
	LeaseID   string        `json:"leaseId"`
	TenantID  string        `json:"tenantId"`
	PoolID    string        `json:"poolId"`
	IPAddress string        `json:"ip"`
	State     string        `json:"state"`
	UpdatedAt time.Time     `json:"updatedAt"`
	Lease     *models.Lease `json:"lease"`
}
