// Package tracing provides OpenTelemetry distributed tracing for Snowplane.
// When enabled via --enable-tracing, all reconcile operations emit distributed
// traces that can be collected by any OTLP-compatible backend (Jaeger, Tempo, etc.).
package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	// TracerName is the instrumentation name used for all Snowplane spans.
	TracerName = "snowplane"

	// DefaultEndpoint is the default OTLP gRPC endpoint.
	DefaultEndpoint = "localhost:4317"
)

// Config holds tracing configuration.
type Config struct {
	// Enabled controls whether tracing is active.
	Enabled bool

	// Endpoint is the OTLP gRPC collector endpoint (e.g. "localhost:4317").
	Endpoint string

	// SamplingRatio is the fraction of traces to sample (0.0–1.0).
	// A value of 1.0 means sample everything.
	SamplingRatio float64

	// Insecure disables TLS for the OTLP exporter connection.
	Insecure bool
}

// Provider wraps the OpenTelemetry TracerProvider and provides a convenient
// shutdown function.
type Provider struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
}

// Setup initialises the global OpenTelemetry TracerProvider.
// If tracing is disabled, it installs a no-op provider and returns a Provider
// whose Shutdown is safe to call.
func Setup(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		noopTP := noop.NewTracerProvider()
		otel.SetTracerProvider(noopTP)

		return &Provider{tracer: noopTP.Tracer(TracerName)}, nil
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	sampler := sdktrace.AlwaysSample()
	if cfg.SamplingRatio > 0 && cfg.SamplingRatio < 1.0 {
		sampler = sdktrace.TraceIDRatioBased(cfg.SamplingRatio)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("snowplane"),
			semconv.ServiceVersion("dev"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(TracerName),
	}, nil
}

// Shutdown flushes any pending spans and shuts down the provider.
// Safe to call on a no-op provider.
func (p *Provider) Shutdown() {
	if p.tp == nil {
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = p.tp.Shutdown(shutdownCtx)
}

// Tracer returns the named tracer for creating spans.
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// ---------------------------------------------------------------------------
// Span helper functions for instrumentation
// ---------------------------------------------------------------------------

// StartSpan starts a new span with the given name and returns the updated
// context and span. The caller must call span.End() when done.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(TracerName).Start(ctx, name,
		trace.WithAttributes(attrs...),
	)
}

// ReconcileAttrs returns common attributes for reconcile spans.
func ReconcileAttrs(resourceType, namespace, name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("snowplane.resource.type", resourceType),
		attribute.String("k8s.namespace", namespace),
		attribute.String("k8s.name", name),
	}
}

// RecordError records an error on the current span without ending it.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err)
	}
}
