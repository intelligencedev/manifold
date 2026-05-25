package agent

import (
	"context"

	"manifold/internal/llm"
	"manifold/internal/llm/budget"
	"manifold/internal/observability"
)

// enforceContextBudget is the last line of defense before dispatching a
// request to the model: it truncates pathological per-message blobs (typically
// tool outputs) and, if the total still exceeds the model context window minus
// a reserve buffer, drops the oldest non-essential messages.
//
// This complements maybeSummarize, which decides what to summarize but always
// keeps a configured tail. A single oversized tool message in that tail can
// still overflow the provider; this helper guarantees it cannot.
func (e *Engine) enforceContextBudget(ctx context.Context, msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	ctxSize := e.ContextWindowTokens
	if ctxSize <= 0 {
		if sz, _ := llm.ContextSize(e.model()); sz > 0 {
			ctxSize = sz
		}
	}
	if ctxSize <= 0 {
		return msgs
	}

	reserve := e.SummaryReserveBufferTokens
	if reserve <= 0 {
		reserve = budget.DefaultReserveBuffer
	}
	if reserve >= ctxSize {
		reserve = ctxSize / 2
	}

	// Per-message rune cap derived from the configured summary chunk size.
	// Falls back to a generous default so well-behaved messages are not
	// touched.
	perMsgRunes := budget.DefaultPerMsgRunes
	if e.SummaryMaxSummaryChunkTokens > 0 {
		perMsgRunes = e.SummaryMaxSummaryChunkTokens * 4
	}

	before := budget.EstimateTokens(msgs)
	out := budget.FitWithProtectedPrefix(msgs, cacheBoundaryPrefixEnd(msgs), ctxSize, reserve, perMsgRunes)
	after := budget.EstimateTokens(out)

	if after < before {
		observability.LoggerWithTrace(ctx).Warn().
			Int("messages_before", len(msgs)).
			Int("messages_after", len(out)).
			Int("tokens_before", before).
			Int("tokens_after", after).
			Int("context_window", ctxSize).
			Int("reserve_buffer", reserve).
			Int("per_msg_runes", perMsgRunes).
			Msg("context_budget_enforced")
	}
	return out
}
