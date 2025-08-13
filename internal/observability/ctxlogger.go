package observability

import (
    "context"

    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
    "go.opentelemetry.io/otel/trace"
)

// LoggerWithTrace returns a zerolog.Logger enriched with trace_id/span_id from the context, if available.
func LoggerWithTrace(ctx context.Context) *zerolog.Logger {
    l := log.Logger
    if ctx == nil {
        return &l
    }
    if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
        l = l.With().Str("trace_id", sc.TraceID().String()).Logger()
        if sc.HasSpanID() {
            l = l.With().Str("span_id", sc.SpanID().String()).Logger()
        }
        if sc.IsSampled() {
            l = l.With().Bool("trace_sampled", true).Logger()
        }
    }
    return &l
}

