package agent

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type OTELTracer struct {
	tracer trace.Tracer
}

func NewOTELTracer() *OTELTracer {
	return &OTELTracer{tracer: otel.Tracer("agent")}
}

func (t *OTELTracer) Start(ctx context.Context, name string, attrs map[string]any) (context.Context, func(err error)) {
	kvs := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		kvs = append(kvs, attribute.String(k, fmtSprint(v)))
	}
	ctx, span := t.tracer.Start(ctx, name, trace.WithAttributes(kvs...))
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}
}

func fmtSprint(v any) string { return fmt.Sprint(v) }
