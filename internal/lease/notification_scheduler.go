package lease

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// NotificationJob describes a scheduled notification for a lease or prefix.
type NotificationJob struct {
	LeaseID   string            `json:"leaseId"`
	TenantID  string            `json:"tenantId"`
	PoolID    string            `json:"poolId"`
	SendAfter time.Time         `json:"sendAfter"`
	Lead      time.Duration     `json:"lead"`
	Kind      string            `json:"kind"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NotificationScheduler enqueues notification jobs for downstream processing.
type NotificationScheduler interface {
	ScheduleLeaseNotification(ctx context.Context, job NotificationJob) error
	Close(ctx context.Context) error
}

// LoggingScheduler is a simple scheduler that logs scheduled jobs.
type LoggingScheduler struct {
	logger *zap.Logger
}

// NewLoggingScheduler builds a scheduler that only logs notifications.
func NewLoggingScheduler(logger *zap.Logger) *LoggingScheduler {
	return &LoggingScheduler{logger: logger}
}

// ScheduleLeaseNotification logs job scheduling, acting as a stub queue.
func (l *LoggingScheduler) ScheduleLeaseNotification(_ context.Context, job NotificationJob) error {
	if l == nil || l.logger == nil {
		return nil
	}
	l.logger.Info("lease notification enqueued",
		zap.String("leaseId", job.LeaseID),
		zap.String("tenantId", job.TenantID),
		zap.String("kind", job.Kind),
		zap.Time("sendAfter", job.SendAfter))
	return nil
}

// Close implements NotificationScheduler.
func (l *LoggingScheduler) Close(context.Context) error {
	return nil
}

// KafkaNotificationScheduler pushes notification jobs to Kafka.
type KafkaNotificationScheduler struct {
	writer *kafka.Writer
	logger *zap.Logger
}

// NewKafkaNotificationScheduler builds a scheduler backed by Kafka.
func NewKafkaNotificationScheduler(brokers []string, topic string, clientID string, logger *zap.Logger) *KafkaNotificationScheduler {
	if len(brokers) == 0 || topic == "" {
		return nil
	}
	cfg := kafka.WriterConfig{
		Brokers:      brokers,
		Topic:        topic,
		RequiredAcks: int(kafka.RequireAll),
		Balancer:     &kafka.Hash{},
	}
	if clientID != "" {
		cfg.Dialer = &kafka.Dialer{ClientID: clientID, Timeout: 10 * time.Second}
	}
	writer := kafka.NewWriter(cfg)
	return &KafkaNotificationScheduler{writer: writer, logger: logger}
}

// ScheduleLeaseNotification publishes a job to Kafka.
func (k *KafkaNotificationScheduler) ScheduleLeaseNotification(ctx context.Context, job NotificationJob) error {
	if k == nil || k.writer == nil {
		return nil
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	key := job.LeaseID
	if key == "" && job.Metadata != nil {
		if prefix, ok := job.Metadata["prefix"]; ok {
			key = prefix
		}
	}
	msg := kafka.Message{Key: []byte(key), Value: payload, Time: time.Now().UTC()}
	return k.writer.WriteMessages(ctx, msg)
}

// Close flushes and closes the Kafka writer.
func (k *KafkaNotificationScheduler) Close(ctx context.Context) error {
	if k == nil || k.writer == nil {
		return nil
	}
	closeCh := make(chan error, 1)
	go func() {
		closeCh <- k.writer.Close()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-closeCh:
		if err != nil && k.logger != nil {
			k.logger.Warn("kafka scheduler close", zap.Error(err))
		}
		return err
	}
}
