package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// ResponsesTokenizer implements llm.Tokenizer using the OpenAI Responses API
// /v1/responses/input_tokens preflight endpoint for accurate token counting.
type ResponsesTokenizer struct {
	client *Client
	model  string
	cache  *llm.TokenCache
}

// NewResponsesTokenizer creates a tokenizer that uses the Responses API input_tokens endpoint.
// The model parameter specifies which model to count tokens for (different models may tokenize differently).
func NewResponsesTokenizer(client *Client, model string, cache *llm.TokenCache) *ResponsesTokenizer {
	return &ResponsesTokenizer{
		client: client,
		model:  model,
		cache:  cache,
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
// This uses the /v1/responses/input_tokens endpoint for accurate counting.
func (t *ResponsesTokenizer) CountMessagesTokens(ctx context.Context, msgs []llm.Message) (int, error) {
	if len(msgs) == 0 {
		return 0, nil
	}

	log := observability.LoggerWithTrace(ctx)

	// Build input items from messages
	input, instructions := t.buildInputItems(msgs)

	req := inputTokensRequest{
		Model: t.model,
		Input: input,
	}
	if strings.TrimSpace(instructions) != "" {
		req.Instructions = instructions
	}

	body, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal input_tokens request: %w", err)
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(t.client.baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	url := baseURL + "/responses/input_tokens"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create input_tokens request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+t.client.apiKey)

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
		log.Warn().
			Int("status", resp.StatusCode).
			Str("body", string(respBody)).
			Msg("input_tokens_api_error")
		return 0, fmt.Errorf("input_tokens returned status %d: %s", resp.StatusCode, string(respBody))
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

// buildInputItems converts llm.Message slice to Responses API input format.
func (t *ResponsesTokenizer) buildInputItems(msgs []llm.Message) ([]any, string) {
	validToolCallIDs := make(map[string]struct{}, 8)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(tc.ID) == "" {
				continue
			}
			validToolCallIDs[tc.ID] = struct{}{}
		}
	}

	items := make([]any, 0, len(msgs))
	var instructions string

	for _, m := range msgs {
		switch m.Role {
		case "system":
			// System messages become instructions in Responses API
			instructions = m.Content
		case "user":
			items = append(items, map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": m.Content},
				},
			})
		case "assistant":
			if m.Compaction != nil {
				// Include compaction item
				items = append(items, map[string]any{
					"type":              "compaction",
					"encrypted_content": m.Compaction.EncryptedContent,
				})
			} else if len(m.ToolCalls) > 0 {
				// Assistant message with tool calls
				item := map[string]any{
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
				}
				if m.Content != "" {
					item["content"] = []map[string]any{
						{"type": "output_text", "text": m.Content},
					}
				}
				items = append(items, item)

				// Add function calls
				for _, tc := range m.ToolCalls {
					items = append(items, map[string]any{
						"type":      "function_call",
						"name":      tc.Name,
						"call_id":   tc.ID,
						"arguments": string(tc.Args),
					})
				}
			} else {
				// Plain assistant message
				items = append(items, map[string]any{
					"type":   "message",
					"role":   "assistant",
					"status": "completed",
					"content": []map[string]any{
						{"type": "output_text", "text": m.Content},
					},
				})
			}
		case "tool":
			// Tool response
			toolID := strings.TrimSpace(m.ToolID)
			if toolID == "" {
				continue
			}
			if _, ok := validToolCallIDs[toolID]; !ok {
				continue
			}
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": toolID,
				"output":  m.Content,
			})
		}
	}

	return items, instructions
}

// Ensure ResponsesTokenizer implements llm.Tokenizer
var _ llm.Tokenizer = (*ResponsesTokenizer)(nil)
