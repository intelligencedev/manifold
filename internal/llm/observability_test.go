package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func resetTokenMetricsStateForTest() {
	totalsMu.Lock()
	defer totalsMu.Unlock()
	modelTotals = map[string]struct{ Prompt, Completion int64 }{}
	modelBuckets = map[string]map[int64]*tokenBucket{}
}

func resetTraceMetricsStateForTest() {
	tracesMu.Lock()
	defer tracesMu.Unlock()
	traceRecords = nil
}

func TestTokenTotalsForWindow(t *testing.T) {
	resetTokenMetricsStateForTest()
	defer resetTokenMetricsStateForTest()
	resetTraceMetricsStateForTest()
	defer resetTraceMetricsStateForTest()

	base := time.Date(2024, 1, 12, 12, 0, 0, 0, time.UTC)
	prevNow := timeNow
	timeNow = func() time.Time { return base }
	defer func() { timeNow = prevNow }()

	recordTokenMetrics("gpt-5", 100, 50, base.Add(-30*time.Minute))
	recordTokenMetrics("gpt-5", 200, 150, base.Add(-90*time.Minute))
	recordTokenMetrics("gpt-4", 10, 10, base.Add(-10*time.Minute))

	totals, applied := TokenTotalsForWindow(time.Hour)
	if len(totals) != 2 {
		t.Fatalf("expected 2 models in window, got %d", len(totals))
	}
	if totals[0].Model != "gpt-5" || totals[0].Prompt != 100 || totals[0].Completion != 50 {
		t.Fatalf("unexpected totals for gpt-5: %+v", totals[0])
	}
	if totals[1].Model != "gpt-4" || totals[1].Prompt != 10 || totals[1].Completion != 10 {
		t.Fatalf("unexpected totals for gpt-4: %+v", totals[1])
	}
	if applied <= 0 || applied > time.Hour {
		t.Fatalf("expected applied window to be within (0, 1h], got %v", applied)
	}

	totalsAll, appliedAll := TokenTotalsForWindow(0)
	if appliedAll != 0 {
		t.Fatalf("expected zero applied window for all-time totals, got %v", appliedAll)
	}
	if len(totalsAll) != 2 || totalsAll[0].Total != 500 {
		t.Fatalf("unexpected all-time totals: %+v", totalsAll)
	}
}

func TestTokenTotalsRetention(t *testing.T) {
	resetTokenMetricsStateForTest()
	defer resetTokenMetricsStateForTest()
	resetTraceMetricsStateForTest()
	defer resetTraceMetricsStateForTest()

	base := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	prevNow := timeNow
	timeNow = func() time.Time { return base }
	defer func() { timeNow = prevNow }()

	old := base.Add(-60 * 24 * time.Hour)
	recent := base.Add(-2 * time.Hour)

	recordTokenMetrics("gpt-5", 500, 500, old)
	recordTokenMetrics("gpt-5", 100, 100, recent)

	totals, applied := TokenTotalsForWindow(30 * 24 * time.Hour)
	if len(totals) != 1 {
		t.Fatalf("expected single model after retention, got %d", len(totals))
	}
	if totals[0].Total != 200 {
		t.Fatalf("expected only recent totals to remain, got %+v", totals[0])
	}
	if applied <= 0 || applied > 30*24*time.Hour {
		t.Fatalf("unexpected applied window %v", applied)
	}
}

func TestTracesForWindow(t *testing.T) {
	resetTraceMetricsStateForTest()
	defer resetTraceMetricsStateForTest()

	base := time.Date(2024, 4, 10, 10, 0, 0, 0, time.UTC)
	prevNow := timeNow
	timeNow = func() time.Time { return base }
	defer func() { timeNow = prevNow }()

	recordTrace(traceRecord{
		snapshot:   TraceSnapshot{Name: "op-old", Model: "gpt-3", Status: "ok", DurationMillis: 10, Timestamp: base.Add(-2 * time.Hour).Unix()},
		recordedAt: base.Add(-2 * time.Hour),
	})
	recordTrace(traceRecord{
		snapshot:   TraceSnapshot{Name: "op-mid", Model: "gpt-4", Status: "ok", DurationMillis: 20, Timestamp: base.Add(-40 * time.Minute).Unix()},
		recordedAt: base.Add(-40 * time.Minute),
	})
	recordTrace(traceRecord{
		snapshot:   TraceSnapshot{Name: "op-new", Model: "gpt-5", Status: "error", DurationMillis: 30, Timestamp: base.Add(-10 * time.Minute).Unix()},
		recordedAt: base.Add(-10 * time.Minute),
	})

	traces, applied := TracesForWindow(time.Hour, 10)
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces in window, got %d", len(traces))
	}
	if traces[0].Name != "op-new" || traces[1].Name != "op-mid" {
		t.Fatalf("unexpected trace order: %+v", traces)
	}
	if applied <= 0 || applied > time.Hour {
		t.Fatalf("expected applied window within (0, 1h], got %v", applied)
	}

	allTraces, appliedAll := TracesForWindow(0, 5)
	if appliedAll != 0 {
		t.Fatalf("expected zero applied window for unlimited query, got %v", appliedAll)
	}
	if len(allTraces) != 3 {
		t.Fatalf("expected all retained traces, got %d", len(allTraces))
	}
}

func TestTraceRetentionAndLimit(t *testing.T) {
	resetTraceMetricsStateForTest()
	defer resetTraceMetricsStateForTest()

	base := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	prevNow := timeNow
	timeNow = func() time.Time { return base }
	defer func() { timeNow = prevNow }()

	// Add an old trace that should be evicted by retention.
	recordTrace(traceRecord{
		snapshot:   TraceSnapshot{Name: "too-old", Model: "gpt-3", Status: "ok", DurationMillis: 1, Timestamp: base.Add(-traceRetention - time.Hour).Unix()},
		recordedAt: base.Add(-traceRetention - time.Hour),
	})

	for i := 0; i < maxTraceEntries+10; i++ {
		recTime := base.Add(-time.Duration(i) * time.Minute)
		recordTrace(traceRecord{
			snapshot: TraceSnapshot{
				Name:           fmt.Sprintf("op-%d", i),
				Model:          "gpt-4",
				Status:         "ok",
				DurationMillis: int64(i),
				Timestamp:      recTime.Unix(),
			},
			recordedAt: recTime,
		})
	}

	if len(traceRecords) != maxTraceEntries {
		t.Fatalf("expected trace records capped to %d, got %d", maxTraceEntries, len(traceRecords))
	}

	traces, _ := TracesForWindow(24*time.Hour, 5)
	if len(traces) != 5 {
		t.Fatalf("expected limit to apply, got %d traces", len(traces))
	}
	if traces[0].Name == "too-old" {
		t.Fatalf("expected old trace to be evicted by retention")
	}
}

func TestLogRedactedPromptSetsRawAttributeOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	exporter := tracetest.NewInMemoryExporter()
	provider := newTestTracerProvider(exporter)
	defer func() { _ = provider.Shutdown(context.Background()) }()

	prevProvider := tracerProviderForTest(provider)
	defer tracerProviderForTest(prevProvider)

	msgs := []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hello"}}

	ConfigureLogging(true, false, 0)
	ctx, span := StartRequestSpan(context.Background(), "chat", "gpt-5", 0, len(msgs))
	LogRedactedPrompt(ctx, msgs)
	span.SetStatus(codes.Ok, "")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if _, ok := findSpanAttr(spans[0].Attributes, "llm.prompt_raw"); ok {
		t.Fatal("did not expect raw prompt attribute when raw prompt logging is disabled")
	}

	exporter.Reset()
	ConfigureLogging(true, true, 0)
	ctx, span = StartRequestSpan(context.Background(), "chat", "gpt-5", 0, len(msgs))
	LogRedactedPrompt(ctx, msgs)
	span.SetStatus(codes.Ok, "")
	span.End()

	spans = exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span after reset, got %d", len(spans))
	}
	raw, ok := findSpanAttr(spans[0].Attributes, "llm.prompt_raw")
	if !ok {
		t.Fatal("expected raw prompt attribute when raw prompt logging is enabled")
	}
	var decoded []Message
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("expected raw prompt payload to be valid JSON, got error: %v", err)
	}
	if len(decoded) != 2 || decoded[0].Content != "sys" || decoded[1].Content != "hello" {
		t.Fatalf("unexpected raw prompt payload: %+v", decoded)
	}

	ConfigureLogging(false, false, 0)
}

func newTestTracerProvider(exporter *tracetest.InMemoryExporter) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
}

func tracerProviderForTest(provider *sdktrace.TracerProvider) *sdktrace.TracerProvider {
	current := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	if existing, ok := current.(*sdktrace.TracerProvider); ok {
		return existing
	}
	return nil
}

func findSpanAttr(attrs []attribute.KeyValue, key string) (string, bool) {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsString(), true
		}
	}
	return "", false
}
