package events

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// WebhookPublisher POSTs policy events to configured endpoints.
type WebhookPublisher struct {
	targets []string
	client  *http.Client
	logger  *zap.Logger
}

// NewWebhookPublisher builds a webhook publisher with sane defaults.
func NewWebhookPublisher(targets []string, logger *zap.Logger) *WebhookPublisher {
	return &WebhookPublisher{
		targets: targets,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

func (p *WebhookPublisher) Publish(ctx context.Context, evt PolicyEvent) error {
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	for _, target := range p.targets {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("policy webhook failed", zap.String("target", target), zap.Error(err))
			}
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 && p.logger != nil {
			p.logger.Warn("policy webhook non-2xx", zap.String("target", target), zap.Int("status", resp.StatusCode))
		}
	}
	return nil
}

func (p *WebhookPublisher) Close(ctx context.Context) error {
	// nothing to close
	return nil
}
