// Package otelinit sets up OpenTelemetry tracing for every service.
//
// The platform standardizes on OTLP/gRPC because the collector deployment in
// the aegis-observability namespace exposes only the OTLP endpoint. Span
// propagation uses the W3C TraceContext + Baggage combination so traces
// survive the hop from the public REST gateway into gRPC services and out
// over Kafka headers.
package otelinit

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// Resource attribute keys. The OpenTelemetry semantic-conventions package
// versions don't stay in lockstep with the SDK; we hard-code the stable
// keys to keep the dependency simple.
const (
	attrServiceName    = "service.name"
	attrServiceVersion = "service.version"
	attrDeployEnv      = "deployment.environment"
)

// Options configures the OTel tracer provider.
type Options struct {
	// ServiceName is required and becomes service.name on every span.
	ServiceName string
	// ServiceVersion is recorded on every span.
	ServiceVersion string
	// Env is recorded as deployment.environment.
	Env string

	// OTLPEndpoint is the gRPC endpoint for the collector. If empty, tracing
	// is configured but spans are dropped — this lets services start cleanly
	// in environments without a collector.
	OTLPEndpoint string
	// Insecure disables TLS to the collector. Use only inside a mesh that
	// enforces mTLS on the wire (Istio Ambient does, in our default config).
	Insecure bool

	// SamplingRatio in [0,1]. 1.0 is full sampling; production typically
	// runs the head sampler at 0.05 and lets the collector tail-sample.
	SamplingRatio float64
}

// Shutdown gracefully flushes and stops the tracer provider.
type Shutdown func(context.Context) error

// Setup installs a global tracer provider per Options. The returned shutdown
// function must be called before the process exits to flush in-flight spans.
func Setup(ctx context.Context, opts Options) (Shutdown, error) {
	if opts.ServiceName == "" {
		return nil, fmt.Errorf("otelinit: ServiceName is required")
	}
	if opts.SamplingRatio == 0 {
		opts.SamplingRatio = 1.0
	}

	res := resource.NewSchemaless(
		attribute.String(attrServiceName, opts.ServiceName),
		attribute.String(attrServiceVersion, opts.ServiceVersion),
		attribute.String(attrDeployEnv, opts.Env),
	)

	tpOpts := []tracesdk.TracerProviderOption{
		tracesdk.WithResource(res),
		tracesdk.WithSampler(tracesdk.ParentBased(tracesdk.TraceIDRatioBased(opts.SamplingRatio))),
	}

	if opts.OTLPEndpoint != "" {
		clientOpts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(opts.OTLPEndpoint),
			otlptracegrpc.WithTimeout(5 * time.Second),
		}
		if opts.Insecure {
			clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
		}
		exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(clientOpts...))
		if err != nil {
			return nil, fmt.Errorf("otelinit: build exporter: %w", err)
		}
		tpOpts = append(tpOpts,
			tracesdk.WithBatcher(exporter, tracesdk.WithBatchTimeout(2*time.Second)),
		)
	}

	tp := tracesdk.NewTracerProvider(tpOpts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
