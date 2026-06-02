package guard

import (
	"context"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

var (
	macACLCheckTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dhcpv4_mac_acl_check_total",
			Help: "Total MAC ACL checks by action, list type, and request phase.",
		},
		[]string{"action", "list_type", "request_phase"},
	)
	macACLMetricRegisterOnce sync.Once
)

type requestPhaseContextKey struct{}

// RegisterMetrics registers guard-level Prometheus collectors.
func RegisterMetrics() {
	macACLMetricRegisterOnce.Do(func() {
		prometheus.MustRegister(macACLCheckTotal)
	})
}

// WithRequestPhaseContext tags ACL evaluation context with a request phase.
func WithRequestPhaseContext(ctx context.Context, phase string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestPhaseContextKey{}, normalizeACLRequestPhase(phase))
}

func requestPhaseFromContext(ctx context.Context) string {
	if ctx == nil {
		return "request"
	}
	phase, _ := ctx.Value(requestPhaseContextKey{}).(string)
	return normalizeACLRequestPhase(phase)
}

func traceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

func normalizeACLRequestPhase(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "discover":
		return "discover"
	case "renew":
		return "renew"
	case "rebind":
		return "rebind"
	default:
		return "request"
	}
}

func observeMACACLCheck(action, listType, requestPhase string) {
	macACLCheckTotal.WithLabelValues(normalizeACLAction(action), normalizeACLListType(listType), normalizeACLRequestPhase(requestPhase)).Inc()
}

func normalizeACLAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "allow":
		return "allow"
	case "monitor":
		return "monitor"
	default:
		return "block"
	}
}

func normalizeACLListType(listType string) string {
	switch strings.ToLower(strings.TrimSpace(listType)) {
	case "black", "blacklist":
		return "black"
	case "white", "whitelist":
		return "white"
	case "gray", "graylist":
		return "gray"
	default:
		return "none"
	}
}
