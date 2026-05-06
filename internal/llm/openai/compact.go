package openai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	rs "github.com/openai/openai-go/v2/responses"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

type responseCompactRequest struct {
	Model              string `json:"model"`
	Input              any    `json:"input,omitempty"`
	Instructions       string `json:"instructions,omitempty"`
	PreviousResponseID string `json:"previous_response_id,omitempty"`
}

type responseCompactOutput struct {
	Type             string `json:"type"`
	ID               string `json:"id"`
	EncryptedContent string `json:"encrypted_content"`
}

type responseCompactResponse struct {
	ID        string                  `json:"id"`
	Object    string                  `json:"object"`
	CreatedAt int64                   `json:"created_at"`
	Output    []responseCompactOutput `json:"output"`
}

// Compact compresses conversation state using the Responses API compaction endpoint.
func (c *Client) Compact(ctx context.Context, msgs []llm.Message, model string, previous *llm.CompactionItem) (*llm.CompactionItem, error) {
	if c.api != "responses" {
		return nil, fmt.Errorf("responses api required for compaction")
	}
	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI Responses Compact", firstNonEmpty(model, c.model), 0, len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	policy := c.responsesContextPolicy(firstNonEmpty(model, c.model))
	input, instructions := buildCompactionInputWithLimit(msgs, previous, policy.ToolOutputMaxChars)
	req := responseCompactRequest{Model: firstNonEmpty(model, c.model)}
	if len(input) > 0 {
		req.Input = input
	}
	if strings.TrimSpace(instructions) != "" {
		req.Instructions = instructions
	}

	var resp responseCompactResponse
	start := time.Now()
	if err := c.sdk.Post(ctx, "/responses/compact", req, &resp); err != nil {
		log.Error().Err(err).Str("model", req.Model).Dur("duration", time.Since(start)).Msg("responses_compact_error")
		span.RecordError(err)
		return nil, err
	}

	for _, item := range resp.Output {
		if item.Type == "compaction" && strings.TrimSpace(item.EncryptedContent) != "" {
			return &llm.CompactionItem{ID: item.ID, EncryptedContent: item.EncryptedContent}, nil
		}
	}
	return nil, errors.New("responses compact returned no compaction item")
}

func buildCompactionInputWithLimit(msgs []llm.Message, previous *llm.CompactionItem, toolOutputMaxChars int) ([]any, string) {
	items := make([]any, 0, len(msgs)+1)
	if previous != nil && strings.TrimSpace(previous.EncryptedContent) != "" {
		payload := map[string]any{
			"type":              "compaction",
			"encrypted_content": previous.EncryptedContent,
		}
		if strings.TrimSpace(previous.ID) != "" {
			payload["id"] = previous.ID
		}
		items = append(items, payload)
	}

	toolCallIDs := make(map[string]struct{}, 8)
	toolOutputIDs := make(map[string]struct{}, 8)
	for _, m := range msgs {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				callID := strings.TrimSpace(tc.ID)
				if callID == "" {
					continue
				}
				toolCallIDs[callID] = struct{}{}
			}
			continue
		}
		if m.Role == "tool" {
			toolID := strings.TrimSpace(m.ToolID)
			if toolID == "" {
				continue
			}
			toolOutputIDs[toolID] = struct{}{}
		}
	}

	var sys []string
	assistantIndex := 0
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				sys = append(sys, m.Content)
			}
		case "user":
			content := strings.TrimSpace(m.Content)
			if content == "" {
				content = " "
			}
			part := rs.ResponseInputContentParamOfInputText(content)
			items = append(items, rs.ResponseInputItemUnionParam{OfInputMessage: &rs.ResponseInputItemMessageParam{
				Content: rs.ResponseInputMessageContentListParam{part},
				Role:    "user",
			}})
		case "assistant":
			// If the assistant provided tool calls, include those so tool outputs can reference them.
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					callID := strings.TrimSpace(tc.ID)
					if callID == "" {
						continue
					}
					if _, ok := toolOutputIDs[callID]; !ok {
						continue
					}
					args := string(tc.Args)
					items = append(items, rs.ResponseInputItemParamOfFunctionCall(args, callID, tc.Name))
				}
			}
			// Also emit text content if present
			content := strings.TrimSpace(m.Content)
			if content != "" {
				msgID := fmt.Sprintf("msg_%d", assistantIndex+1)
				assistantIndex++
				text := rs.ResponseOutputTextParam{Text: content, Annotations: []rs.ResponseOutputTextAnnotationUnionParam{}}
				contentParts := []rs.ResponseOutputMessageContentUnionParam{{OfOutputText: &text}}
				items = append(items, rs.ResponseInputItemParamOfOutputMessage(contentParts, msgID, rs.ResponseOutputMessageStatusCompleted))
			}
		case "tool":
			out := boundedResponsesToolOutputWithLimit(m.Content, toolOutputMaxChars)
			toolID := strings.TrimSpace(m.ToolID)
			if toolID == "" {
				continue
			}
			if _, ok := toolCallIDs[toolID]; !ok {
				continue
			}
			items = append(items, rs.ResponseInputItemParamOfFunctionCallOutput(toolID, out))
		}
	}

	return items, strings.Join(sys, "\n\n")
}
