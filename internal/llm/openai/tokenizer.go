package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	rs "github.com/openai/openai-go/v3/responses"
	"github.com/rs/zerolog"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// ResponsesTokenizer implements llm.Tokenizer using the OpenAI Responses API
// /v1/responses/input_tokens preflight endpoint for accurate token counting.
type ResponsesTokenizer struct {
	client             *Client
	model              string
	cache              *llm.TokenCache
	toolOutputMaxChars int
}

// NewResponsesTokenizer creates a tokenizer that uses the Responses API input_tokens endpoint.
// The model parameter specifies which model to count tokens for (different models may tokenize differently).
func NewResponsesTokenizer(client *Client, model string, cache *llm.TokenCache, toolOutputMaxChars int) *ResponsesTokenizer {
	if toolOutputMaxChars <= 0 {
		toolOutputMaxChars = maxResponsesToolOutputChars
	}
	return &ResponsesTokenizer{
		client:             client,
		model:              model,
		cache:              cache,
		toolOutputMaxChars: toolOutputMaxChars,
	}
}

// CountTokens counts tokens for a single text string.
func (t *ResponsesTokenizer) CountTokens(ctx context.Context, text string) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}

	if t.cache != nil {
		if count, ok := t.cache.Get(text); ok {
			return count, nil
		}
	}

	msgs := []llm.Message{{Role: "user", Content: text}}
	count, err := t.CountMessagesTokens(ctx, msgs)
	if err != nil {
		return 0, err
	}

	if t.cache != nil {
		t.cache.Set(text, count)
	}

	return count, nil
}

// CountMessagesTokens counts tokens for a conversation (array of messages).
// This uses the /v1/responses/input_tokens endpoint for accurate counting.
func (t *ResponsesTokenizer) CountMessagesTokens(ctx context.Context, msgs []llm.Message) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	if t == nil || t.client == nil {
		return llm.EstimateTokensForMessages(msgs), nil
	}
	if t.client.inputTokensUnsupported.Load() {
		return llm.EstimateTokensForMessages(msgs), nil
	}

	log := observability.LoggerWithTrace(ctx)

	params := t.countParams(msgs)
	result, err := t.client.sdk.Responses.InputTokens.Count(ctx, params)
	if err != nil {
		var apiErr *sdk.Error
		if errors.As(err, &apiErr) {
			return t.handleInputTokensStatus(ctx, log, apiErr.StatusCode, []byte(apiErr.RawJSON()), msgs)
		}
		return 0, fmt.Errorf("input_tokens request: %w", err)
	}
	if result == nil {
		return 0, fmt.Errorf("input_tokens returned nil response")
	}
	totalTokens := int(result.InputTokens)

	log.Debug().
		Int("total_tokens", totalTokens).
		Int("message_count", len(msgs)).
		Msg("input_tokens_counted")

	return totalTokens, nil
}

func (t *ResponsesTokenizer) countParams(msgs []llm.Message) rs.InputTokenCountParams {
	input, instructions := adaptResponsesInputWithLimit(msgs, t.toolOutputMaxChars)
	params := rs.InputTokenCountParams{
		Model: param.NewOpt(t.model),
	}
	if len(input) > 0 {
		params.Input.OfResponseInputItemArray = input
	}
	if strings.TrimSpace(instructions) != "" {
		params.Instructions = param.NewOpt(instructions)
	}
	return params
}

func (t *ResponsesTokenizer) handleInputTokensStatus(ctx context.Context, log *zerolog.Logger, statusCode int, body []byte, msgs []llm.Message) (int, error) {
	if statusCode == http.StatusNotFound {
		return t.handleUnsupportedInputTokens(ctx, log, statusCode, msgs)
	}
	log.Warn().
		Int("status", statusCode).
		Str("body", string(body)).
		Msg("input_tokens_api_error")
	return 0, fmt.Errorf("input_tokens returned status %d: %s", statusCode, string(body))
}

func (t *ResponsesTokenizer) handleUnsupportedInputTokens(ctx context.Context, log *zerolog.Logger, statusCode int, msgs []llm.Message) (int, error) {
	t.client.inputTokensUnsupported.Store(true)
	if t.client.isSelfHosted() {
		local := NewLocalTokenizer(t.client, t.model, t.cache)
		if count, err := local.CountMessagesTokens(ctx, msgs); err == nil {
			log.Debug().
				Int("status", statusCode).
				Int("total_tokens", count).
				Msg("input_tokens_endpoint_unsupported_used_local_tokenizer")
			return count, nil
		}
	}
	log.Debug().
		Int("status", statusCode).
		Msg("input_tokens_endpoint_unsupported_using_heuristic")
	return llm.EstimateTokensForMessages(msgs), nil
}

// Ensure ResponsesTokenizer implements llm.Tokenizer
var _ llm.Tokenizer = (*ResponsesTokenizer)(nil)
