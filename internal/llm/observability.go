package llm

import (
	"context"
	"encoding/json"
	"sync"

	"intelligence.dev/internal/observability"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	mu                   sync.RWMutex
	enablePayloadLogging = false
	truncateBytes        = 0 // 0 means no truncation
)

// --- Token metrics aggregation (exposed to web UI) ---------------------------
var (
	tokenOnce         sync.Once
	promptCounter     otelmetric.Int64Counter
	completionCounter otelmetric.Int64Counter
	totalsMu          sync.RWMutex
	modelTotals       = map[string]struct{ Prompt, Completion int64 }{}
)

// ensureTokenInstruments lazily initializes OTel instruments once a provider
// has been installed (InitOTel should run before first use in normal startup).
func ensureTokenInstruments() {
	tokenOnce.Do(func() {
		m := otel.Meter("internal/llm")
		var err error
		promptCounter, err = m.Int64Counter("llm.prompt_tokens", otelmetric.WithDescription("Cumulative prompt tokens by model"))
		if err != nil {
			// leave zero-value counter (no-op) if creation fails
		}
		completionCounter, err = m.Int64Counter("llm.completion_tokens", otelmetric.WithDescription("Cumulative completion tokens by model"))
		if err != nil {
		}
	})
}

// RecordTokenMetrics records token usage for a model and updates in-process
// cumulative totals used by the /api/metrics/tokens endpoint. This supplements
// OTel export (we can't easily pull data back from the exporter) while still
// leveraging standard metric instruments for external backends.
func RecordTokenMetrics(model string, promptTokens, completionTokens int) {
	if model == "" || (promptTokens == 0 && completionTokens == 0) {
		return
	}
	ensureTokenInstruments()
	ctx := context.Background()
	if promptCounter != nil && promptTokens > 0 {
		promptCounter.Add(ctx, int64(promptTokens), otelmetric.WithAttributes(attribute.String("llm.model", model)))
	}
	if completionCounter != nil && completionTokens > 0 {
		completionCounter.Add(ctx, int64(completionTokens), otelmetric.WithAttributes(attribute.String("llm.model", model)))
	}
	totalsMu.Lock()
	cur := modelTotals[model]
	cur.Prompt += int64(promptTokens)
	cur.Completion += int64(completionTokens)
	modelTotals[model] = cur
	totalsMu.Unlock()
}

// TokenTotal represents cumulative token counts per model since process start.
type TokenTotal struct {
	Model      string `json:"model"`
	Prompt     int64  `json:"prompt"`
	Completion int64  `json:"completion"`
	Total      int64  `json:"total"`
}

// TokenTotalsSnapshot returns a stable snapshot of current cumulative totals.
func TokenTotalsSnapshot() []TokenTotal {
	totalsMu.RLock()
	defer totalsMu.RUnlock()
	out := make([]TokenTotal, 0, len(modelTotals))
	for m, v := range modelTotals {
		out = append(out, TokenTotal{Model: m, Prompt: v.Prompt, Completion: v.Completion, Total: v.Prompt + v.Completion})
	}
	return out
}

// ConfigureLogging sets global behavior for prompt/response logging.
// Call this once at startup with values from the main config.
func ConfigureLogging(enable bool, truncate int) {
	mu.Lock()
	defer mu.Unlock()
	enablePayloadLogging = enable
	truncateBytes = truncate
}

// StartRequestSpan starts a tracer span for an LLM request and sets common attributes.
func StartRequestSpan(ctx context.Context, operation string, model string, tools int, messages int) (context.Context, trace.Span) {
	ctx, span := otel.Tracer("internal/llm").Start(ctx, operation)
	span.SetAttributes(attribute.String("llm.model", model), attribute.Int("llm.tools", tools), attribute.Int("llm.messages", messages))
	return ctx, span
}

func shouldLog() (bool, int) {
	mu.RLock()
	defer mu.RUnlock()
	return enablePayloadLogging, truncateBytes
}

// LogRedactedPrompt logs a redacted copy of the prompt/messages at debug level using the observability helpers.
// If global logging is disabled this is a no-op. Very large payloads are truncated according to configuration.
func LogRedactedPrompt(ctx context.Context, msgs []Message) {
	if ok, t := shouldLog(); !ok {
		return
	} else {
		log := observability.LoggerWithTrace(ctx)
		if b, err := json.Marshal(msgs); err == nil {
			red := observability.RedactJSON(b)
			if t > 0 && len(red) > t {
				previewObj := map[string]any{"truncated": true, "preview": string(red[:t])}
				if pb, err := json.Marshal(previewObj); err == nil {
					tmp := log.With().RawJSON("prompt", pb).Logger()
					tt := &tmp
					tt.Debug().Msg("llm_request")
					return
				}
			}
			tmp := log.With().RawJSON("prompt", red).Logger()
			tt := &tmp
			tt.Debug().Msg("llm_request")
		}
	}
}

// LogRedactedResponse logs a redacted copy of the response payload at debug level.
// If global logging is disabled this is a no-op. Very large payloads are truncated according to configuration.
func LogRedactedResponse(ctx context.Context, resp any) {
	if ok, t := shouldLog(); !ok {
		return
	} else {
		log := observability.LoggerWithTrace(ctx)
		if b, err := json.Marshal(resp); err == nil {
			red := observability.RedactJSON(b)
			if t > 0 && len(red) > t {
				previewObj := map[string]any{"truncated": true, "preview": string(red[:t])}
				if pb, err := json.Marshal(previewObj); err == nil {
					tmp := log.With().RawJSON("response", pb).Logger()
					tt := &tmp
					tt.Debug().Msg("llm_response")
					return
				}
			}
			tmp := log.With().RawJSON("response", red).Logger()
			tt := &tmp
			tt.Debug().Msg("llm_response")
		}
	}
}

// RecordTokenAttributes sets token count attributes on the provided span.
func RecordTokenAttributes(span trace.Span, promptTokens, completionTokens, totalTokens int) {
	if span == nil {
		return
	}
	span.SetAttributes(attribute.Int("llm.prompt_tokens", promptTokens), attribute.Int("llm.completion_tokens", completionTokens), attribute.Int("llm.total_tokens", totalTokens))
	// Also record as metrics / aggregate for UI
	if modelAttr := span.SpanContext().TraceID(); modelAttr.IsValid() {
		// We don't actually have the model stored here; model is an attribute on span
		// so attempt to fetch it via span attributes isn't available post-hoc. Caller
		// should record metrics directly if they want model breakdown. This function
		// only updates OTel attributes; metric aggregation done at call-sites where
		// model string is available. (No-op here.)
	}
}
