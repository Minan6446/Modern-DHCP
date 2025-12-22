package ipsgdai

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// HTTPPublisherOptions configures the HTTP publisher.
type HTTPPublisherOptions struct {
	URL    string
	Secret string
	Retry  int
	Client *http.Client
	Logger *zap.Logger
}

type httpPublisher struct {
	url    string
	secret string
	retry  int
	client *http.Client
	logger *zap.Logger
}

// NewHTTPPublisher creates a publisher that POSTs lease snapshots to an HTTPS endpoint.
func NewHTTPPublisher(opts HTTPPublisherOptions) (Publisher, error) {
	if opts.URL == "" {
		return nil, errors.New("ipsgdai: publish URL required")
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	retry := opts.Retry
	if retry <= 0 {
		retry = 3
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &httpPublisher{url: opts.URL, secret: opts.Secret, retry: retry, client: client, logger: logger}, nil
}

func (p *httpPublisher) Publish(ctx context.Context, snapshot LeaseSnapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	attempts := p.retry
	for attempts > 0 {
		attempts--
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if p.secret != "" {
			mac := hmac.New(sha256.New, []byte(p.secret))
			mac.Write(payload)
			req.Header.Set("X-IPSG-Signature", hex.EncodeToString(mac.Sum(nil)))
		}

		resp, err := p.client.Do(req)
		if err != nil {
			if attempts == 0 {
				return err
			}
			p.logger.Debug("ipsg publish retry", zap.Error(err))
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < 300 {
			return nil
		}
		if attempts == 0 {
			return errors.New("ipsgdai: publish failed")
		}
		p.logger.Debug("ipsg publish non-2xx", zap.Int("status", resp.StatusCode))
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
