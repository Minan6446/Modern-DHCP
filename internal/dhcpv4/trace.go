package dhcpv4

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func dhcpv4Logger(ctx context.Context, base *zap.Logger) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return base.With(zap.String("traceId", ""))
	}
	return base.With(zap.String("traceId", spanCtx.TraceID().String()))
}
