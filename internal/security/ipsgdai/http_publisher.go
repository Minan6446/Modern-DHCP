package ipsgdai

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

	backoff := 100 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < p.retry; attempt++ {
		reader := bytes.NewReader(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, reader)
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
		if err == nil {
			if resp.Body != nil {
				_ = resp.Body.Close()
			}
			if resp.StatusCode >= http.StatusOK && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("ipsgdai: publish failed with status %d", resp.StatusCode)
			p.logger.Debug("ipsg publish non-2xx", zap.Int("status", resp.StatusCode))
		} else {
			lastErr = err
			p.logger.Debug("ipsg publish retry", zap.Error(err))
		}

		if attempt == p.retry-1 {
			break
		}

		wait := backoff * time.Duration(1<<attempt)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		return errors.New("ipsgdai: publish failed")
	}
	return lastErr
}
