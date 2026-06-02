package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"modern-dhcp/internal/alerting"
	"modern-dhcp/internal/config"
	"modern-dhcp/internal/monitoring"
)

type runtimeStoreStub struct {
	thresholds alerting.Thresholds
	notify     alerting.NotifyConfig
	templates  []alerting.Template
	receivers  []alerting.Receiver
}

func (s *runtimeStoreStub) GetThresholds(_ context.Context, _ string) (alerting.Thresholds, error) {
	return s.thresholds, nil
}

func (s *runtimeStoreStub) GetNotify(_ context.Context, _ string) (alerting.NotifyConfig, error) {
	return s.notify, nil
}

func (s *runtimeStoreStub) ListTemplates(_ context.Context, _ string) ([]alerting.Template, error) {
	return append([]alerting.Template(nil), s.templates...), nil
}

func (s *runtimeStoreStub) ListReceivers(_ context.Context, _ string) ([]alerting.Receiver, error) {
	return append([]alerting.Receiver(nil), s.receivers...), nil
}

type captureNotifier struct {
	name   string
	mu     sync.Mutex
	events []alerting.Event
}

func (n *captureNotifier) Name() string { return n.name }

func (n *captureNotifier) Notify(_ context.Context, event alerting.Event) error {
	n.mu.Lock()
	n.events = append(n.events, event)
	n.mu.Unlock()
	return nil
}

func (n *captureNotifier) Snapshot() []alerting.Event {
	n.mu.Lock()
	defer n.mu.Unlock()
	dup := make([]alerting.Event, len(n.events))
	copy(dup, n.events)
	return dup
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	requestTracker := monitoring.NewRequestTracker()
	rateTracker := monitoring.NewRateLimitTracker(256)
	aggregator := monitoring.NewAggregator(monitoring.Options{
		RequestTracker: requestTracker,
		RateTracker:    rateTracker,
		SecurityWindow: 2 * time.Minute,
	})

	// Inject request samples to trigger failure-ratio/latency based alerts.
	for i := 0; i < 12; i++ {
		requestTracker.ObserveRequest("global", "dhcpv4", "RENEW", i < 2, 120*time.Millisecond)
	}

	store := &runtimeStoreStub{
		thresholds: alerting.Thresholds{
			TenantID:                   "global",
			ResourceFailedRequestRatio: 30,
			ResourceRenewFail:          30,
			ServerResponseTimeMs:       80,
			NetworkAbnormalQPS:         1,
			UpdatedAt:                  time.Now().UTC(),
		},
		notify: alerting.NotifyConfig{
			TenantID:   "global",
			WebhookURL: "https://hooks.example.com/runtime-e2e",
			Channels: alerting.Channels{
				Email:   true,
				SMS:     true,
				Webhook: true,
			},
			Policies: alerting.Policies{
				Emergency: []string{"email", "webhook"},
				Critical:  []string{"email", "webhook"},
				Info:      []string{"sms"},
			},
		},
		templates: []alerting.Template{
			{
				ID:       "tpl-e2e-email",
				TenantID: "global",
				Lang:     "zh-CN",
				Channel:  "email",
				Subject:  "[E2E] {{category}}/{{severity}}",
				Body:     "tenant={{tenantId}} rule={{ruleName}} details={{details}}",
			},
		},
		receivers: []alerting.Receiver{
			{
				ID:       "receiver-1",
				TenantID: "global",
				Name:     "Ops",
				Email:    "ops@example.com",
				Phone:    "13800000000",
				Levels:   []string{"critical", "emergency", "all"},
			},
		},
	}

	manager := alerting.NewManager(alerting.ManagerOptions{DedupeWindow: 100 * time.Millisecond})
	emailSink := &captureNotifier{name: "email"}
	smsSink := &captureNotifier{name: "sms"}
	webhookSink := &captureNotifier{name: "webhook"}
	manager.RegisterNotifier("email", emailSink)
	manager.RegisterNotifier("sms", smsSink)
	manager.RegisterNotifier("webhook", webhookSink)

	enabled := true
	controller := monitoring.NewAlertController(monitoring.AlertControllerOptions{
		Config: config.AlertingConfig{
			Enabled:          true,
			EvaluateInterval: 120 * time.Millisecond,
			DefaultPolicy: config.AlertPolicyConfig{
				Enabled:       &enabled,
				SilenceWindow: 0,
			},
		},
		Aggregator:  aggregator,
		RateTracker: rateTracker,
		ConfigStore: store,
		Manager:     manager,
	})

	controller.Start(ctx)
	time.Sleep(450 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	emailEvents := emailSink.Snapshot()
	if len(emailEvents) == 0 {
		fatalf("未捕获到 email 通道告警事件")
	}

	var matched bool
	for _, event := range emailEvents {
		if !strings.Contains(event.Summary, "[E2E]") {
			continue
		}
		if event.Labels["receiver_emails"] != "ops@example.com" {
			continue
		}
		if event.Labels["receiver_phones"] != "13800000000" {
			continue
		}
		if event.Labels["webhook_url"] != "https://hooks.example.com/runtime-e2e" {
			continue
		}
		if !strings.Contains(event.Details, "tenant=global") {
			continue
		}
		matched = true
		break
	}
	if !matched {
		fatalf("email 事件未命中模板渲染/接收人注入断言")
	}

	fmt.Println("[PASS] alerting e2e self-test passed")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[FAIL] "+format+"\n", args...)
	os.Exit(1)
}
