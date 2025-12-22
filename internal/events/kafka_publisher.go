package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// KafkaPublisher writes policy events to a Kafka topic.
type KafkaPublisher struct {
	writer *kafka.Writer
	logger *zap.Logger
}

// NewKafkaPublisher creates a Kafka-based publisher.
func NewKafkaPublisher(brokers []string, topic string, logger *zap.Logger) *KafkaPublisher {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireAll,
		Async:        true,
	}
	return &KafkaPublisher{writer: writer, logger: logger}
}

func (p *KafkaPublisher) Publish(ctx context.Context, evt PolicyEvent) error {
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := kafka.Message{
		Key:   []byte(evt.RuleID),
		Value: payload,
		Time:  time.Now().UTC(),
	}
	return p.writer.WriteMessages(ctx, msg)
}

func (p *KafkaPublisher) Close(ctx context.Context) error {
	if err := p.writer.Close(); err != nil {
		if p.logger != nil {
			p.logger.Warn("kafka publisher close", zap.Error(err))
		}
		return err
	}
	return nil
}
