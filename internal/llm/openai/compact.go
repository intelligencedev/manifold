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
		items = append(items, previousCompactionPayload(previous))
	}

	toolCallIDs, toolOutputIDs := compactionToolIDs(msgs)
	var sys []string
	state := compactionInputState{
		items:         items,
		toolCallIDs:   toolCallIDs,
		toolOutputIDs: toolOutputIDs,
	}
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				sys = append(sys, m.Content)
			}
		case "user":
			state.appendUserMessage(m)
		case "assistant":
			state.appendAssistantMessage(m)
		case "tool":
			state.appendToolMessage(m, toolOutputMaxChars)
		}
	}

	return state.items, strings.Join(sys, "\n\n")
}

type compactionInputState struct {
	items          []any
	toolCallIDs    map[string]struct{}
	toolOutputIDs  map[string]struct{}
	assistantIndex int
}

func previousCompactionPayload(previous *llm.CompactionItem) map[string]any {
	payload := map[string]any{
		"type":              "compaction",
		"encrypted_content": previous.EncryptedContent,
	}
	if strings.TrimSpace(previous.ID) != "" {
		payload["id"] = previous.ID
	}
	return payload
}

func compactionToolIDs(msgs []llm.Message) (map[string]struct{}, map[string]struct{}) {
	toolCallIDs := make(map[string]struct{}, 8)
	toolOutputIDs := make(map[string]struct{}, 8)
	for _, m := range msgs {
		switch {
		case m.Role == "assistant" && len(m.ToolCalls) > 0:
			addCompactionToolCallIDs(toolCallIDs, m)
		case m.Role == "tool":
			addCompactionToolOutputID(toolOutputIDs, m)
		}
	}
	return toolCallIDs, toolOutputIDs
}

func addCompactionToolCallIDs(ids map[string]struct{}, msg llm.Message) {
	for _, tc := range msg.ToolCalls {
		callID := strings.TrimSpace(tc.ID)
		if callID != "" {
			ids[callID] = struct{}{}
		}
	}
}

func addCompactionToolOutputID(ids map[string]struct{}, msg llm.Message) {
	toolID := strings.TrimSpace(msg.ToolID)
	if toolID != "" {
		ids[toolID] = struct{}{}
	}
}

func (s *compactionInputState) appendUserMessage(msg llm.Message) {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		content = " "
	}
	part := rs.ResponseInputContentParamOfInputText(content)
	s.items = append(s.items, rs.ResponseInputItemUnionParam{OfInputMessage: &rs.ResponseInputItemMessageParam{
		Content: rs.ResponseInputMessageContentListParam{part},
		Role:    "user",
	}})
}

func (s *compactionInputState) appendAssistantMessage(msg llm.Message) {
	s.appendAssistantToolCalls(msg)
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return
	}
	s.assistantIndex++
	msgID := fmt.Sprintf("msg_%d", s.assistantIndex)
	text := rs.ResponseOutputTextParam{Text: content, Annotations: []rs.ResponseOutputTextAnnotationUnionParam{}}
	contentParts := []rs.ResponseOutputMessageContentUnionParam{{OfOutputText: &text}}
	s.items = append(s.items, rs.ResponseInputItemParamOfOutputMessage(contentParts, msgID, rs.ResponseOutputMessageStatusCompleted))
}

func (s *compactionInputState) appendAssistantToolCalls(msg llm.Message) {
	for _, tc := range msg.ToolCalls {
		callID := strings.TrimSpace(tc.ID)
		if callID == "" {
			continue
		}
		if _, ok := s.toolOutputIDs[callID]; !ok {
			continue
		}
		s.items = append(s.items, rs.ResponseInputItemParamOfFunctionCall(string(tc.Args), callID, tc.Name))
	}
}

func (s *compactionInputState) appendToolMessage(msg llm.Message, toolOutputMaxChars int) {
	toolID := strings.TrimSpace(msg.ToolID)
	if toolID == "" {
		return
	}
	if _, ok := s.toolCallIDs[toolID]; !ok {
		return
	}
	out := boundedResponsesToolOutputWithLimit(msg.Content, toolOutputMaxChars)
	s.items = append(s.items, rs.ResponseInputItemParamOfFunctionCallOutput(toolID, out))
}
