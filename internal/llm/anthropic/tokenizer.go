package anthropic

import (
	"context"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// MessagesTokenizer implements llm.Tokenizer using the Anthropic Messages API
// /v1/messages/count_tokens endpoint for accurate preflight token counting.
type MessagesTokenizer struct {
	sdk   anthropic.Client
	model string
	cache *llm.TokenCache
}

// NewMessagesTokenizer creates a tokenizer that uses the Messages API count_tokens endpoint.
// The model parameter specifies which model to count tokens for.
func NewMessagesTokenizer(sdk anthropic.Client, model string, cache *llm.TokenCache) *MessagesTokenizer {
	return &MessagesTokenizer{
		sdk:   sdk,
		model: model,
		cache: cache,
	}
}

// CountTokens counts tokens for a single text string.
func (t *MessagesTokenizer) CountTokens(ctx context.Context, text string) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}

	// Check cache first
	if t.cache != nil {
		if count, ok := t.cache.Get(text); ok {
			return count, nil
		}
	}

	// Build a simple user message for counting
	msgs := []llm.Message{{Role: "user", Content: text}}
	count, err := t.CountMessagesTokens(ctx, msgs)
	if err != nil {
		return 0, err
	}

	// Cache the result
	if t.cache != nil {
		t.cache.Set(text, count)
	}

	return count, nil
}

// CountMessagesTokens counts tokens for a conversation (array of messages).
// This uses the /v1/messages/count_tokens endpoint for accurate counting.
func (t *MessagesTokenizer) CountMessagesTokens(ctx context.Context, msgs []llm.Message) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}

	log := observability.LoggerWithTrace(ctx)

	// Convert llm.Message to Anthropic MessageParam format
	apiMsgs, system := t.buildMessageParams(msgs)

	params := anthropic.MessageCountTokensParams{
		Messages: apiMsgs,
		Model:    anthropic.Model(t.model),
	}

	// Add system prompt if present
	if strings.TrimSpace(system) != "" {
		params.System = anthropic.MessageCountTokensParamsSystemUnion{
			OfString: anthropic.String(system),
		}
	}

	result, err := t.sdk.Messages.CountTokens(ctx, params)
	if err != nil {
		log.Warn().
			Err(err).
			Str("model", t.model).
			Int("messages", len(msgs)).
			Msg("anthropic_count_tokens_error")
		return 0, err
	}

	log.Debug().
		Int64("input_tokens", result.InputTokens).
		Int("message_count", len(msgs)).
		Msg("anthropic_count_tokens_ok")

	return int(result.InputTokens), nil
}

// buildMessageParams converts llm.Message slice to Anthropic API message params format.
func (t *MessagesTokenizer) buildMessageParams(msgs []llm.Message) ([]anthropic.MessageParam, string) {
	params := make([]anthropic.MessageParam, 0, len(msgs))
	var system string

	for _, m := range msgs {
		switch m.Role {
		case "system":
			// System messages are handled separately in Anthropic API
			system = m.Content
		case "user":
			if strings.TrimSpace(m.Content) != "" {
				params = append(params, anthropic.NewUserMessage(
					anthropic.NewTextBlock(m.Content),
				))
			}
		case "assistant":
			if len(m.ToolCalls) > 0 {
				// Assistant message with tool calls
				blocks := []anthropic.ContentBlockParamUnion{}
				if strings.TrimSpace(m.Content) != "" {
					blocks = append(blocks, anthropic.NewTextBlock(m.Content))
				}
				for _, tc := range m.ToolCalls {
					blocks = append(blocks, anthropic.NewToolUseBlock(
						tc.ID,
						decodeArgs(tc.Args),
						tc.Name,
					))
				}
				if len(blocks) > 0 {
					params = append(params, anthropic.NewAssistantMessage(blocks...))
				}
			} else if strings.TrimSpace(m.Content) != "" {
				// Plain assistant message
				params = append(params, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(m.Content),
				))
			}
		case "tool":
			// Tool result messages
			params = append(params, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(m.ToolID, m.Content, false),
			))
		}
	}

	return params, system
}

// Ensure MessagesTokenizer implements llm.Tokenizer
var _ llm.Tokenizer = (*MessagesTokenizer)(nil)
