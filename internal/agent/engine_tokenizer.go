package agent

import (
	"context"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

func (e *Engine) AttachTokenizer(provider any, cache *llm.TokenCache) {
	if e == nil || provider == nil {
		return
	}

	type tokenizableProvider interface {
		Tokenizer(cache *llm.TokenCache) llm.Tokenizer
	}

	p, ok := provider.(tokenizableProvider)
	if !ok {
		return
	}

	if tok := p.Tokenizer(cache); tok != nil {
		e.Tokenizer = tok
		// Log when we have to fall back so we can spot API failures without breaking runs.
		e.TokenizationFallbackToHeuristic = true
	}
}

// countTokens returns the token count for text using the engine's tokenizer if available,
// otherwise falls back to heuristic estimation.
func (e *Engine) countTokens(ctx context.Context, text string) int {
	if e.Tokenizer == nil {
		return llm.EstimateTokens(text)
	}
	count, err := e.Tokenizer.CountTokens(ctx, text)
	if err != nil {
		if e.TokenizationFallbackToHeuristic {
			observability.LoggerWithTrace(ctx).Debug().
				Err(err).
				Msg("tokenization_failed_using_heuristic")
			return llm.EstimateTokens(text)
		}
		// Return heuristic anyway if we can't tokenize
		return llm.EstimateTokens(text)
	}
	return count
}

// countMessagesTokens returns the token count for a slice of messages using the engine's
// tokenizer if available, otherwise falls back to heuristic estimation.
func (e *Engine) countMessagesTokens(ctx context.Context, msgs []llm.Message) int {
	if e.Tokenizer == nil {
		return llm.EstimateTokensForMessages(msgs)
	}
	count, err := e.Tokenizer.CountMessagesTokens(ctx, msgs)
	if err != nil {
		if e.TokenizationFallbackToHeuristic {
			observability.LoggerWithTrace(ctx).Debug().
				Err(err).
				Msg("tokenization_failed_using_heuristic")
			return llm.EstimateTokensForMessages(msgs)
		}
		// Return heuristic anyway if we can't tokenize
		return llm.EstimateTokensForMessages(msgs)
	}
	return count
}
