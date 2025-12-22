package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/notifications"
)

// NotificationFanoutPayload describes the expected payload for notification fanout jobs.
type NotificationFanoutPayload struct {
	Channels []string          `json:"channels"`
	Topic    string            `json:"topic"`
	Summary  string            `json:"summary"`
	Severity string            `json:"severity"`
	Metadata map[string]string `json:"metadata"`
	Body     map[string]any    `json:"body"`
}

// NotificationFanoutHandler dispatches automation jobs to the notifications subsystem.
type NotificationFanoutHandler struct {
	dispatcher      *notifications.Dispatcher
	defaultChannels []string
	logger          *zap.Logger
}

// NewNotificationFanoutHandler creates a handler that bridges the scheduler with the dispatcher.
func NewNotificationFanoutHandler(dispatcher *notifications.Dispatcher, logger *zap.Logger, defaultChannels []string) *NotificationFanoutHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &NotificationFanoutHandler{dispatcher: dispatcher, logger: logger, defaultChannels: append([]string(nil), defaultChannels...)}
}

// Handle implements automation.Handler.
func (h *NotificationFanoutHandler) Handle(ctx context.Context, job Job) error {
	if h == nil || h.dispatcher == nil {
		return errors.New("automation: notification dispatcher unavailable")
	}

	payload := NotificationFanoutPayload{}
	if len(job.Payload) > 0 {
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			h.logger.Warn("invalid notification payload", zap.String("jobId", job.ID), zap.Error(err))
		}
	}
	channels := payload.Channels
	if len(channels) == 0 {
		channels = h.defaultChannels
	}
	if len(channels) == 0 {
		return errors.New("automation: no notification channels configured")
	}

	msg := notifications.Message{
		ID:        job.ID,
		TenantID:  job.TenantID,
		Topic:     payload.Topic,
		Summary:   payload.Summary,
		Severity:  payload.Severity,
		Metadata:  payload.Metadata,
		Body:      payload.Body,
		CreatedAt: job.CreatedAt,
		Source:    string(job.Type),
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if msg.Topic == "" {
		msg.Topic = string(job.Type)
	}
	if msg.Summary == "" {
		msg.Summary = fmt.Sprintf("automation job %s", job.Type)
	}
	if msg.Severity == "" {
		msg.Severity = "info"
	}
	if msg.Body == nil && len(job.Payload) > 0 {
		var generic map[string]any
		if err := json.Unmarshal(job.Payload, &generic); err == nil {
			msg.Body = generic
		}
	}

	if err := h.dispatcher.Dispatch(ctx, msg, channels...); err != nil {
		return err
	}
	return nil
}
