package llm

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"manifold/internal/observability"

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
	modelBuckets      = map[string]map[int64]*tokenBucket{}
)

const (
	tokenBucketResolution = time.Minute
	tokenBucketRetention  = 45 * 24 * time.Hour
)

type tokenBucket struct {
	Prompt     int64
	Completion int64
}

var timeNow = time.Now

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
	recordTokenMetrics(model, promptTokens, completionTokens, timeNow())
}

func recordTokenMetrics(model string, promptTokens, completionTokens int, ts time.Time) {
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
	ts = ts.UTC()
	p := int64(promptTokens)
	c := int64(completionTokens)
	totalsMu.Lock()
	cur := modelTotals[model]
	cur.Prompt += p
	cur.Completion += c
	modelTotals[model] = cur
	if p > 0 || c > 0 {
		updateTokenBucketsLocked(model, ts, p, c)
	}
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
	for model, v := range modelTotals {
		out = append(out, TokenTotal{Model: model, Prompt: v.Prompt, Completion: v.Completion, Total: v.Prompt + v.Completion})
	}
	sortTokenTotals(out)
	return out
}

// TokenTotalsForWindow returns token aggregates limited to the requested
// window. When the requested window exceeds in-memory retention, the applied
// window in the return value reflects the actually covered duration.
func TokenTotalsForWindow(window time.Duration) ([]TokenTotal, time.Duration) {
	totalsMu.RLock()
	defer totalsMu.RUnlock()

	if window <= 0 {
		out := make([]TokenTotal, 0, len(modelTotals))
		for model, v := range modelTotals {
			out = append(out, TokenTotal{Model: model, Prompt: v.Prompt, Completion: v.Completion, Total: v.Prompt + v.Completion})
		}
		sortTokenTotals(out)
		return out, 0
	}

	now := timeNow().UTC()
	cutoffKey := bucketKey(now.Add(-window))

	out := make([]TokenTotal, 0, len(modelBuckets))
	var earliestIncludedKey int64
	hasIncluded := false

	for model, buckets := range modelBuckets {
		var prompt, completion int64
		for key, bucket := range buckets {
			if key < cutoffKey {
				continue
			}
			prompt += bucket.Prompt
			completion += bucket.Completion
			if !hasIncluded || key < earliestIncludedKey {
				earliestIncludedKey = key
				hasIncluded = true
			}
		}
		if prompt == 0 && completion == 0 {
			continue
		}
		out = append(out, TokenTotal{
			Model:      model,
			Prompt:     prompt,
			Completion: completion,
			Total:      prompt + completion,
		})
	}
	sortTokenTotals(out)

	applied := window
	if hasIncluded {
		nowKey := bucketKey(now)
		if nowKey >= earliestIncludedKey {
			available := time.Duration(nowKey-earliestIncludedKey)*time.Second + tokenBucketResolution
			if available < applied {
				applied = available
			}
		}
	}

	return out, applied
}

func updateTokenBucketsLocked(model string, ts time.Time, prompt, completion int64) {
	key := bucketKey(ts)
	buckets := modelBuckets[model]
	if buckets == nil {
		buckets = make(map[int64]*tokenBucket)
		modelBuckets[model] = buckets
	}
	bucket := buckets[key]
	if bucket == nil {
		bucket = &tokenBucket{}
		buckets[key] = bucket
	}
	bucket.Prompt += prompt
	bucket.Completion += completion

	cutoffKey := bucketKey(ts.Add(-tokenBucketRetention))
	for k := range buckets {
		if k < cutoffKey {
			delete(buckets, k)
		}
	}

	if len(buckets) == 0 {
		delete(modelBuckets, model)
	}
}

func bucketKey(ts time.Time) int64 {
	return ts.Truncate(tokenBucketResolution).Unix()
}

func sortTokenTotals(totals []TokenTotal) {
	sort.Slice(totals, func(i, j int) bool {
		if totals[i].Total == totals[j].Total {
			return totals[i].Model < totals[j].Model
		}
		return totals[i].Total > totals[j].Total
	})
}

func resetTokenMetricsState() {
	totalsMu.Lock()
	defer totalsMu.Unlock()
	modelTotals = map[string]struct{ Prompt, Completion int64 }{}
	modelBuckets = map[string]map[int64]*tokenBucket{}
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
