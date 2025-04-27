package agent

import (
	"context"
)

// NullTracer is a no-op tracer for tests.
type NullTracer struct{}

func (t *NullTracer) Start(_ context.Context, _ string, _ map[string]any) (context.Context, func(err error)) {
	return context.Background(), func(err error) {}
}
