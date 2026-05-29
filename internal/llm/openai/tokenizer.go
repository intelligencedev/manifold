package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// inputTokensRequest represents the request body for /v1/responses/input_tokens
type inputTokensRequest struct {
	Model        string `json:"model"`
	Input        []any  `json:"input"`
	Instructions string `json:"instructions,omitempty"`
}

// inputTokensResponse represents the response from /v1/responses/input_tokens
type inputTokensResponse struct {
	TotalTokens int `json:"total_tokens"`
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

	httpReq, err := t.newInputTokensRequest(ctx, msgs)
	if err != nil {
		return 0, err
	}

	resp, err := t.client.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("input_tokens request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read input_tokens response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return t.handleInputTokensStatus(ctx, log, resp.StatusCode, respBody, msgs)
	}

	var result inputTokensResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, fmt.Errorf("unmarshal input_tokens response: %w", err)
	}

	log.Debug().
		Int("total_tokens", result.TotalTokens).
		Int("message_count", len(msgs)).
		Msg("input_tokens_counted")

	return result.TotalTokens, nil
}

func (t *ResponsesTokenizer) newInputTokensRequest(ctx context.Context, msgs []llm.Message) (*http.Request, error) {
	input, instructions := t.buildInputItems(msgs)
	req := inputTokensRequest{Model: t.model, Input: input}
	if strings.TrimSpace(instructions) != "" {
		req.Instructions = instructions
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal input_tokens request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, inputTokensURL(t.client.baseURL), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create input_tokens request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	t.client.applyAuthHeader(httpReq)
	return httpReq, nil
}

func inputTokensURL(baseURL string) string {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return baseURL + "/responses/input_tokens"
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

// buildInputItems converts llm.Message slice to Responses API input format.
func (t *ResponsesTokenizer) buildInputItems(msgs []llm.Message) ([]any, string) {
	validToolCallIDs := collectValidToolCallIDs(msgs)
	items := make([]any, 0, len(msgs))
	var instructions string

	for _, m := range msgs {
		switch m.Role {
		case "system":
			instructions = m.Content
		case "user":
			items = append(items, userInputItem(m.Content))
		case "assistant":
			items = append(items, assistantInputItems(m)...)
		case "tool":
			if item, ok := toolOutputInputItem(m, validToolCallIDs, t.toolOutputMaxChars); ok {
				items = append(items, item)
			}
		}
	}

	return items, instructions
}

func collectValidToolCallIDs(msgs []llm.Message) map[string]struct{} {
	ids := make(map[string]struct{}, 8)
	for _, msg := range msgs {
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}
		for _, call := range msg.ToolCalls {
			if id := strings.TrimSpace(call.ID); id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	return ids
}

func userInputItem(content string) map[string]any {
	content = strings.TrimSpace(content)
	if content == "" {
		content = " "
	}
	return map[string]any{
		"type": "message",
		"role": "user",
		"content": []map[string]any{
			{"type": "input_text", "text": content},
		},
	}
}

func assistantInputItems(msg llm.Message) []any {
	if msg.Compaction != nil {
		return []any{map[string]any{
			"type":              "compaction",
			"encrypted_content": msg.Compaction.EncryptedContent,
		}}
	}
	if len(msg.ToolCalls) > 0 {
		return assistantToolCallItems(msg)
	}
	if item, ok := assistantTextItem(msg.Content); ok {
		return []any{item}
	}
	return nil
}

func assistantToolCallItems(msg llm.Message) []any {
	items := make([]any, 0, len(msg.ToolCalls)+1)
	if item, ok := assistantTextItem(msg.Content); ok {
		items = append(items, item)
	}
	for _, call := range msg.ToolCalls {
		items = append(items, map[string]any{
			"type":      "function_call",
			"name":      call.Name,
			"call_id":   call.ID,
			"arguments": string(call.Args),
		})
	}
	return items
}

func assistantTextItem(content string) (map[string]any, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false
	}
	return map[string]any{
		"type":   "message",
		"role":   "assistant",
		"status": "completed",
		"content": []map[string]any{
			{"type": "output_text", "text": content},
		},
	}, true
}

func toolOutputInputItem(msg llm.Message, validToolCallIDs map[string]struct{}, maxChars int) (map[string]any, bool) {
	toolID := strings.TrimSpace(msg.ToolID)
	if toolID == "" {
		return nil, false
	}
	if _, ok := validToolCallIDs[toolID]; !ok {
		return nil, false
	}
	return map[string]any{
		"type":    "function_call_output",
		"call_id": toolID,
		"output":  boundedResponsesToolOutputWithLimit(msg.Content, maxChars),
	}, true
}

// Ensure ResponsesTokenizer implements llm.Tokenizer
var _ llm.Tokenizer = (*ResponsesTokenizer)(nil)
