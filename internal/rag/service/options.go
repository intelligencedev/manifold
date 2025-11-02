package service

import (
    "context"
    "time"
)

// Clock abstracts time to make the service testable.
type Clock interface {
    Now() time.Time
}

// SystemClock implements Clock using time.Now.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// Logger is a minimal logging interface satisfied by zerolog and others.
type Logger interface {
    // Info logs structured messages at info level.
    Info(msg string, fields map[string]any)
    // Error logs structured messages at error level.
    Error(msg string, fields map[string]any)
    // Debug logs structured messages at debug level.
    Debug(msg string, fields map[string]any)
}

// Metrics is a placeholder for observability counters/histograms.
// We keep it minimal for scaffolding; concrete impl can adapt to Prometheus/Otel.
type Metrics interface {
    IncCounter(name string, labels map[string]string)
    ObserveHistogram(name string, value float64, labels map[string]string)
}

// NoopMetrics implements Metrics without side effects.
type NoopMetrics struct{}

func (NoopMetrics) IncCounter(string, map[string]string)               {}
func (NoopMetrics) ObserveHistogram(string, float64, map[string]string) {}

// CtxKey typed context key for request-scoped values.
type CtxKey string

// WithTenant returns a context that carries the tenant identifier.
func WithTenant(ctx context.Context, tenant string) context.Context {
    if tenant == "" {
        return ctx
    }
    return context.WithValue(ctx, CtxKey("tenant"), tenant)
}

