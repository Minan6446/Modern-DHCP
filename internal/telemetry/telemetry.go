package telemetry

import (
	context "context"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Options configure telemetry wiring.
type Options struct {
	Enabled      bool
	ServiceName  string
	Environment  string
	OTLPEndpoint string
	Insecure     bool
	Headers      map[string]string
	SamplerRatio float64
}

// Provider holds tracer/meter providers and a shutdown hook.
type Provider struct {
	TracerProvider *tracesdk.TracerProvider
	MeterProvider  *metricsdk.MeterProvider
	Tracer         trace.Tracer
	Meter          metric.Meter
	Shutdown       func(context.Context) error
}

type silentOTelErrorHandler struct{}

func (silentOTelErrorHandler) Handle(error) {}

// Setup configures global OTel providers. It is safe to call with opts.Enabled=false (returns noop providers).
func Setup(ctx context.Context, opts Options) (*Provider, error) {
	otel.SetErrorHandler(silentOTelErrorHandler{})
	if !opts.Enabled {
		noop := &Provider{
			TracerProvider: tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.NeverSample())),
			MeterProvider:  metricsdk.NewMeterProvider(),
			Tracer:         otel.Tracer("noop"),
			Meter:          otel.Meter("noop"),
			Shutdown:       func(context.Context) error { return nil },
		}
		otel.SetTracerProvider(noop.TracerProvider)
		otel.SetMeterProvider(noop.MeterProvider)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
		return noop, nil
	}

	if opts.SamplerRatio <= 0 || opts.SamplerRatio > 1 {
		opts.SamplerRatio = 0.1
	}
	if opts.ServiceName == "" {
		opts.ServiceName = "modern-dhcp"
	}
	opts.OTLPEndpoint, opts.Insecure = normalizeOTLPEndpoint(opts.OTLPEndpoint, opts.Insecure)
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(opts.ServiceName),
			semconv.DeploymentEnvironment(opts.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	traceClientOpts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(opts.OTLPEndpoint)}
	if opts.Insecure {
		traceClientOpts = append(traceClientOpts, otlptracegrpc.WithInsecure())
	}
	if len(opts.Headers) > 0 {
		traceClientOpts = append(traceClientOpts, otlptracegrpc.WithHeaders(opts.Headers))
	}
	traceExp, err := otlptracegrpc.New(ctx, traceClientOpts...)
	if err != nil {
		return nil, err
	}

	metricClientOpts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(opts.OTLPEndpoint)}
	if opts.Insecure {
		metricClientOpts = append(metricClientOpts, otlpmetricgrpc.WithInsecure())
	}
	if len(opts.Headers) > 0 {
		metricClientOpts = append(metricClientOpts, otlpmetricgrpc.WithHeaders(opts.Headers))
	}
	metricExp, err := otlpmetricgrpc.New(ctx, metricClientOpts...)
	if err != nil {
		return nil, err
	}

	samplers := tracesdk.ParentBased(tracesdk.TraceIDRatioBased(opts.SamplerRatio))
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(samplers),
		tracesdk.WithBatcher(traceExp, batchOptions()...),
		tracesdk.WithResource(res),
	)

	mp := metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricsdk.NewPeriodicReader(metricExp)),
		metricsdk.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	provider := &Provider{
		TracerProvider: tp,
		MeterProvider:  mp,
		Tracer:         tp.Tracer(opts.ServiceName),
		Meter:          mp.Meter(opts.ServiceName),
	}
	provider.Shutdown = func(ctx context.Context) error {
		if err := mp.Shutdown(ctx); err != nil {
			return err
		}
		return tp.Shutdown(ctx)
	}
	return provider, nil
}

func normalizeOTLPEndpoint(endpoint string, insecure bool) (string, bool) {
	v := strings.TrimSpace(endpoint)
	if v == "" {
		return v, insecure
	}
	if strings.Contains(v, "://") {
		if parsed, err := url.Parse(v); err == nil {
			if strings.EqualFold(strings.TrimSpace(parsed.Scheme), "http") {
				insecure = true
			}
			if host := strings.TrimSpace(parsed.Host); host != "" {
				v = host
			}
		}
	}
	if idx := strings.Index(v, "/"); idx >= 0 {
		v = strings.TrimSpace(v[:idx])
	}
	return v, insecure
}

func batchOptions() []tracesdk.BatchSpanProcessorOption {
	return []tracesdk.BatchSpanProcessorOption{
		tracesdk.WithMaxExportBatchSize(512),
		tracesdk.WithBatchTimeout(5 * time.Second),
		tracesdk.WithMaxQueueSize(4096),
	}
}
