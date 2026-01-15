package observability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"manifold/internal/config"

	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// InitOTel configures tracing and metrics exporters. Returns a shutdown func.
func InitOTel(ctx context.Context, obs config.ObsConfig) (func(context.Context) error, error) {
	if obs.OTLP == "" {
		return nil, errors.New("otlp endpoint is required")
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithAttributes(
			semconv.ServiceName(obs.ServiceName),
			semconv.ServiceVersion(obs.ServiceVersion),
			attribute.String("deployment.environment", obs.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("init resource: %w", err)
	}

	trExp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(obs.OTLP), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("init trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(trExp),
		sdktrace.WithResource(res),
	)

	mExp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(obs.OTLP), otlpmetrichttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("init metrics exporter: %w", err)
	}
	reader := metric.NewPeriodicReader(mExp, metric.WithInterval(10*time.Second))
	mp := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if err := host.Start(host.WithMeterProvider(mp)); err != nil {
		return nil, fmt.Errorf("failed to start host metrics: %w", err)
	}

	return func(ctx context.Context) error {
		var first error
		if err := mp.Shutdown(ctx); err != nil {
			first = err
		}
		if err := tp.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
		return first
	}, nil
}
