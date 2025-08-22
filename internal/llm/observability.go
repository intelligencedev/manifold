package llm

import (
	"context"
	"encoding/json"
	"sync"

	"singularityio/internal/observability"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	mu                   sync.RWMutex
	enablePayloadLogging = false
	truncateBytes        = 0 // 0 means no truncation
)

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
}
