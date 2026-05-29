package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"manifold/internal/llm"
	"manifold/internal/observability"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

func (c *Client) chatStreamSSEFallback(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatStream (SSE Fallback)", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)

	payload, err := c.chatStreamSSEPayload(msgs, tools, model)
	if err != nil {
		return err
	}
	resp, err := c.postChatStreamSSE(ctx, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := validateChatStreamSSEResponse(resp, log); err != nil {
		return err
	}

	start := time.Now()
	state := newChatStreamSSEState(h)
	scanErr := scanChatStreamSSE(resp.Body, state)
	c.recordChatStreamSSEMetrics(ctx, span, msgs, model, state)

	dur := time.Since(start)
	if scanErr != nil && !errors.Is(scanErr, context.Canceled) {
		observability.LoggerWithTrace(ctx).Error().Err(scanErr).Dur("duration", dur).Msg("chat_stream_sse_fallback_error")
		span.RecordError(scanErr)
		return scanErr
	}
	observability.LoggerWithTrace(ctx).Debug().Dur("duration", dur).Msg("chat_stream_sse_fallback_ok")
	return nil
}

func (c *Client) chatStreamSSEPayload(msgs []llm.Message, tools []llm.ToolSchema, model string) ([]byte, error) {
	body := map[string]any{"model": firstNonEmpty(model, c.model), "messages": AdaptMessages(model, c.chatCompletionMessages(msgs)), "stream": true}
	actualTools := configureChatCompletionBodyTools(body, tools, c.isSelfHosted())
	if len(c.extra) > 0 {
		tmp := make(map[string]any, len(c.extra))
		maps.Copy(tmp, c.extra)
		if !actualTools {
			delete(tmp, "parallel_tool_calls")
		}
		for k, v := range tmp {
			if k != "model" && k != "messages" && k != "stream" {
				body[k] = v
			}
		}
	}
	streamOptions := map[string]any{"include_usage": true}
	if existing, ok := body["stream_options"].(map[string]any); ok {
		maps.Copy(streamOptions, existing)
	} else if existing, ok := body["streamOptions"].(map[string]any); ok {
		maps.Copy(streamOptions, existing)
	}
	body["stream_options"] = streamOptions
	delete(body, "streamOptions")
	return json.Marshal(body)
}

func (c *Client) postChatStreamSSE(ctx context.Context, payload []byte) (*http.Response, error) {
	base := strings.TrimSuffix(strings.TrimSpace(c.baseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	c.applyAuthHeader(req)
	return c.httpClient.Do(req)
}

func validateChatStreamSSEResponse(resp *http.Response, log *zerolog.Logger) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	log.Error().Int("status", resp.StatusCode).RawJSON("body", observability.RedactJSON(body)).Msg("sse_fallback_bad_status")
	return fmt.Errorf("chatStream SSE fallback: status %d", resp.StatusCode)
}

type chatStreamSSEState struct {
	h                        llm.StreamHandler
	assistantContent         strings.Builder
	reportedPromptTokens     int
	reportedCompletionTokens int
	reportedTotalTokens      int
	toolCalls                map[int]*llm.ToolCall
	toolCallsFlushed         bool
	thoughtStream            selfHostedThoughtStreamState
}

func newChatStreamSSEState(h llm.StreamHandler) *chatStreamSSEState {
	return &chatStreamSSEState{h: h, toolCalls: map[int]*llm.ToolCall{}}
}

func scanChatStreamSSE(body io.Reader, state *chatStreamSSEState) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		data, ok := chatStreamSSEData(scanner.Text())
		if !ok {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			continue
		}
		state.handlePayload(m)
	}
	return scanner.Err()
}

func chatStreamSSEData(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}

func (s *chatStreamSSEState) handlePayload(m map[string]any) {
	if usage, ok := m["usage"].(map[string]any); ok {
		s.reportedPromptTokens = usageInt(usage["prompt_tokens"])
		s.reportedCompletionTokens = usageInt(usage["completion_tokens"])
		s.reportedTotalTokens = usageInt(usage["total_tokens"])
		if s.reportedTotalTokens == 0 {
			s.reportedTotalTokens = s.reportedPromptTokens + s.reportedCompletionTokens
		}
	}
	if choices, ok := m["choices"].([]any); ok && len(choices) > 0 {
		s.handleChoice(choices[0])
		return
	}
	if value, ok := m["response"].(string); ok {
		s.emitDelta(value)
	}
	if value, ok := m["token"].(string); ok {
		s.emitDelta(value)
	}
}

func (s *chatStreamSSEState) handleChoice(raw any) {
	choice, ok := raw.(map[string]any)
	if !ok {
		return
	}
	if delta, ok := choice["delta"].(map[string]any); ok {
		s.handleDelta(delta)
	}
	if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" && !s.toolCallsFlushed {
		s.flushToolCalls()
	}
	if msg, ok := choice["message"].(map[string]any); ok {
		s.handleMessage(msg)
	}
}

func (s *chatStreamSSEState) handleDelta(delta map[string]any) {
	s.emitReasoning(delta["reasoning_content"])
	s.emitReasoning(delta["reasoningContent"])
	if content, ok := delta["content"].(string); ok {
		s.emitDelta(content)
	}
	if toolCalls, ok := delta["tool_calls"].([]any); ok {
		s.accumulateToolCalls(toolCalls)
	}
}

func (s *chatStreamSSEState) handleMessage(msg map[string]any) {
	s.emitReasoning(msg["reasoning_content"])
	s.emitReasoning(msg["reasoningContent"])
	if content, ok := msg["content"].(string); ok {
		s.emitDelta(content)
	}
}

func (s *chatStreamSSEState) emitReasoning(raw any) {
	if reasoning := extractSelfHostedReasoningContent(raw); reasoning != "" {
		s.thoughtStream.emitReasoning(reasoning, s.h)
	}
}

func (s *chatStreamSSEState) emitDelta(content string) {
	if content == "" {
		return
	}
	s.thoughtStream.emit(content, s.h, &s.assistantContent)
}

func (s *chatStreamSSEState) accumulateToolCalls(toolCalls []any) {
	for i, raw := range toolCalls {
		if raw == nil {
			continue
		}
		if s.toolCalls[i] == nil {
			s.toolCalls[i] = &llm.ToolCall{}
		}
		if toolCall, ok := raw.(map[string]any); ok {
			s.accumulateToolCall(i, toolCall)
		}
	}
}

func (s *chatStreamSSEState) accumulateToolCall(i int, toolCall map[string]any) {
	if id, ok := toolCall["id"].(string); ok && id != "" {
		s.toolCalls[i].ID = id
	}
	fn, ok := toolCall["function"].(map[string]any)
	if !ok {
		return
	}
	if name, ok := fn["name"].(string); ok && name != "" {
		s.toolCalls[i].Name = name
	}
	if args, ok := fn["arguments"].(string); ok && args != "" {
		if s.toolCalls[i].Args == nil {
			s.toolCalls[i].Args = json.RawMessage(args)
		} else {
			s.toolCalls[i].Args = json.RawMessage(string(s.toolCalls[i].Args) + args)
		}
	}
}

func (s *chatStreamSSEState) flushToolCalls() {
	for _, tc := range s.toolCalls {
		if tc != nil && tc.Name != "" && len(tc.Args) > 0 {
			s.h.OnToolCall(*tc)
		}
	}
	s.toolCallsFlushed = true
}

func (c *Client) recordChatStreamSSEMetrics(ctx context.Context, span trace.Span, msgs []llm.Message, model string, state *chatStreamSSEState) {
	if !c.isSelfHosted() {
		return
	}
	promptTokens := state.reportedPromptTokens
	completionTokens := state.reportedCompletionTokens
	totalTokens := state.reportedTotalTokens
	if promptTokens == 0 && completionTokens == 0 && totalTokens == 0 {
		promptTokens = c.tokenizeCount(ctx, buildPromptText(msgs))
		completionTokens = c.tokenizeCount(ctx, state.assistantContent.String())
		totalTokens = promptTokens + completionTokens
	}
	llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
	if promptTokens > 0 || completionTokens > 0 {
		llm.RecordTokenMetricsFromContext(ctx, firstNonEmpty(model, c.model), promptTokens, completionTokens)
	}
	llm.LogRedactedResponse(ctx, map[string]int{"prompt_tokens": promptTokens, "completion_tokens": completionTokens, "total_tokens": totalTokens})
}

func usageInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}
