package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// HTTPClient abstracts http.Client for easier testing.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// WebhookOptions configures a WebhookSender instance.
type WebhookOptions struct {
	Secret  string
	Method  string
	Headers map[string]string
	Timeout time.Duration
	Client  HTTPClient
}

// WebhookSender posts notification messages to an HTTP endpoint.
type WebhookSender struct {
	endpoint string
	method   string
	secret   string
	headers  map[string]string
	client   HTTPClient
	logger   *zap.Logger
}

// NewWebhookSender creates a webhook sender.
func NewWebhookSender(endpoint string, opts WebhookOptions, logger *zap.Logger) *WebhookSender {
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = http.MethodPost
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	headers := make(map[string]string, len(opts.Headers))
	for k, v := range opts.Headers {
		headers[k] = v
	}
	return &WebhookSender{
		endpoint: endpoint,
		method:   method,
		secret:   opts.Secret,
		headers:  headers,
		client:   client,
		logger:   logger,
	}
}

// Send posts the message JSON to the configured endpoint.
func (w *WebhookSender) Send(ctx context.Context, msg Message) error {
	if _, err := url.ParseRequestURI(w.endpoint); err != nil {
		return err
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, w.method, w.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.secret != "" {
		req.Header.Set("X-Notification-Secret", w.secret)
	}
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webhook status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	w.logger.Debug("notification webhook delivered", zap.String("endpoint", w.endpoint))
	return nil
}
