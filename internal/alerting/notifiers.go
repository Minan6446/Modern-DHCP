package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// HTTPClient abstracts http.Client for easier testing.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient is used by webhook/chat notifiers when the caller does not provide one.
var DefaultHTTPClient HTTPClient = &http.Client{Timeout: 5 * time.Second}

// EmailNotifier renders alerts via SMTP-compatible bridges.
type EmailNotifier struct {
	name       string
	recipients []string
	from       string
	logger     *zap.Logger
}

func NewEmailNotifier(name, from string, recipients []string, logger *zap.Logger) *EmailNotifier {
	return &EmailNotifier{name: strings.ToLower(name), from: from, recipients: recipients, logger: logger}
}

func (n *EmailNotifier) Name() string { return n.name }

func (n *EmailNotifier) Notify(_ context.Context, event Event) error {
	if len(n.recipients) == 0 {
		return errors.New("email notifier missing recipients")
	}
	if n.logger != nil {
		n.logger.Info("email alert", zap.Strings("to", n.recipients), zap.String("subject", event.Summary), zap.String("severity", string(event.Severity)))
	}
	return nil
}

// SMSNotifier proxies alerts to SMS providers.
type SMSNotifier struct {
	name    string
	numbers []string
	logger  *zap.Logger
}

func NewSMSNotifier(name string, numbers []string, logger *zap.Logger) *SMSNotifier {
	return &SMSNotifier{name: strings.ToLower(name), numbers: numbers, logger: logger}
}

func (n *SMSNotifier) Name() string { return n.name }

func (n *SMSNotifier) Notify(_ context.Context, event Event) error {
	if len(n.numbers) == 0 {
		return errors.New("sms notifier missing numbers")
	}
	if n.logger != nil {
		n.logger.Info("sms alert", zap.Strings("numbers", n.numbers), zap.String("summary", event.Summary))
	}
	return nil
}

// VoiceNotifier escalates events through voice/phone trees.
type VoiceNotifier struct {
	name    string
	numbers []string
	logger  *zap.Logger
}

func NewVoiceNotifier(name string, numbers []string, logger *zap.Logger) *VoiceNotifier {
	return &VoiceNotifier{name: strings.ToLower(name), numbers: numbers, logger: logger}
}

func (n *VoiceNotifier) Name() string { return n.name }

func (n *VoiceNotifier) Notify(_ context.Context, event Event) error {
	if len(n.numbers) == 0 {
		return errors.New("voice notifier missing numbers")
	}
	if n.logger != nil {
		n.logger.Info("voice alert", zap.Strings("numbers", n.numbers), zap.String("summary", event.Summary))
	}
	return nil
}

// ChatNotifier posts alerts to chat platforms (Slack/Teams/钉钉/企业微信等).
type ChatNotifier struct {
	name    string
	channel string
	webhook string
	client  HTTPClient
	logger  *zap.Logger
}

func NewChatNotifier(name, channel, webhook string, client HTTPClient, logger *zap.Logger) *ChatNotifier {
	if client == nil {
		client = DefaultHTTPClient
	}
	return &ChatNotifier{name: strings.ToLower(name), channel: channel, webhook: webhook, client: client, logger: logger}
}

func (n *ChatNotifier) Name() string { return n.name }

func (n *ChatNotifier) Notify(ctx context.Context, event Event) error {
	if n.client == nil {
		return errors.New("chat notifier missing client")
	}
	if _, err := url.Parse(n.webhook); err != nil {
		return err
	}
	payload := map[string]any{
		"channel":  n.channel,
		"severity": event.Severity,
		"summary":  event.Summary,
		"details":  event.Details,
		"tenant":   event.TenantID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("chat webhook returned %d", resp.StatusCode)
	}
	if n.logger != nil {
		n.logger.Info("chat alert delivered", zap.String("channel", n.channel), zap.String("webhook", n.webhook))
	}
	return nil
}

// WebhookNotifier posts JSON payloads to downstream systems.
type WebhookNotifier struct {
	name   string
	url    string
	secret string
	client HTTPClient
	logger *zap.Logger
}

func NewWebhookNotifier(name, targetURL, secret string, client HTTPClient, logger *zap.Logger) *WebhookNotifier {
	if client == nil {
		client = DefaultHTTPClient
	}
	return &WebhookNotifier{name: strings.ToLower(name), url: targetURL, secret: secret, client: client, logger: logger}
}

func (n *WebhookNotifier) Name() string { return n.name }

func (n *WebhookNotifier) Notify(ctx context.Context, event Event) error {
	if n.client == nil {
		return errors.New("webhook notifier missing client")
	}
	if _, err := url.Parse(n.url); err != nil {
		return err
	}
	payload := map[string]any{
		"severity": event.Severity,
		"category": event.Category,
		"summary":  event.Summary,
		"details":  event.Details,
		"tenant":   event.TenantID,
		"labels":   event.Labels,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.secret != "" {
		req.Header.Set("X-Webhook-Secret", n.secret)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with %d", resp.StatusCode)
	}
	if n.logger != nil {
		n.logger.Info("webhook alert delivered", zap.String("url", n.url))
	}
	return nil
}

// SNMPNotifier sends traps to legacy NMS endpoints.
type SNMPNotifier struct {
	name      string
	target    string
	community string
	logger    *zap.Logger
}

func NewSNMPNotifier(name, target, community string, logger *zap.Logger) *SNMPNotifier {
	return &SNMPNotifier{name: strings.ToLower(name), target: target, community: community, logger: logger}
}

func (n *SNMPNotifier) Name() string { return n.name }

func (n *SNMPNotifier) Notify(_ context.Context, event Event) error {
	if n.logger != nil {
		n.logger.Info("snmp trap", zap.String("target", n.target), zap.String("severity", string(event.Severity)))
	}
	return nil
}

// SyslogNotifier forwards alerts to syslog collectors.
type SyslogNotifier struct {
	name    string
	addr    string
	network string
	logger  *zap.Logger
}

func NewSyslogNotifier(name, addr, network string, logger *zap.Logger) *SyslogNotifier {
	if network == "" {
		network = "udp"
	}
	return &SyslogNotifier{name: strings.ToLower(name), addr: addr, network: network, logger: logger}
}

func (n *SyslogNotifier) Name() string { return n.name }

func (n *SyslogNotifier) Notify(_ context.Context, event Event) error {
	if _, err := net.ResolveUDPAddr(n.network, n.addr); err != nil {
		return err
	}
	if n.logger != nil {
		n.logger.Info("syslog alert", zap.String("addr", n.addr), zap.String("severity", string(event.Severity)))
	}
	return nil
}
