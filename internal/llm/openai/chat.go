package openai

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v2"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"manifold/internal/llm"
	"manifold/internal/observability"
)

// Chat implements llm.Provider.Chat using OpenAI Chat Completions.
func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	if imgOpts, ok := llm.ImagePromptFromContext(ctx); ok {
		return c.chatWithImageGeneration(ctx, msgs, model, imgOpts)
	}
	return c.ChatWithOptions(ctx, msgs, tools, model, nil)
}

// ChatWithOptions is like Chat but allows callers to:
//   - omit tools entirely by passing a nil or empty tools slice
//   - inject provider-specific extra fields (e.g., reasoning_effort)
//     via params.WithExtraField.
func (c *Client) ChatWithOptions(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error) {
	tools = c.requestTools(tools)

	if strings.EqualFold(c.api, "responses") {
		return c.chatResponses(ctx, msgs, tools, model, extra)
	}
	log := observability.LoggerWithTrace(ctx)
	// Tracing and prompt logging
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatWithOptions", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	params := c.chatCompletionParams(msgs, tools, model, extra)
	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_error")
		span.RecordError(err)
		return llm.Message{}, err
	}
	c.logChatCompletionOK(log, comp, params, tools, msgs, dur)
	out := c.messageFromChatCompletion(comp, string(params.Model))
	llm.LogRedactedResponse(ctx, comp.Choices)
	c.recordChatCompletionUsage(ctx, span, comp.Usage, string(params.Model), msgs, out.Content)
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	return out, nil
}

func (c *Client) chatCompletionParams(msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) sdk.ChatCompletionNewParams {
	params := sdk.ChatCompletionNewParams{Model: sdk.ChatModel(firstNonEmpty(model, c.model))}
	params.Messages = AdaptMessages(string(params.Model), c.chatCompletionMessages(msgs))
	actualTools := configureChatCompletionTools(&params, tools, c.isSelfHosted())
	if len(c.extra) > 0 || len(extra) > 0 {
		merged := make(map[string]any, len(c.extra)+len(extra))
		maps.Copy(merged, c.extra)
		maps.Copy(merged, extra)
		if !actualTools {
			delete(merged, "parallel_tool_calls")
		}
		params.SetExtraFields(sanitizeExtraFields(merged))
	}
	return params
}

func (c *Client) logChatCompletionOK(
	log *zerolog.Logger,
	comp *sdk.ChatCompletion,
	params sdk.ChatCompletionNewParams,
	tools []llm.ToolSchema,
	msgs []llm.Message,
	dur time.Duration,
) {
	fields := chatCompletionLogEvent(log, comp, params, tools, msgs, dur).Logger()
	if c.logPayloads && c.extra != nil && len(c.extra) > 0 {
		if b, err := json.Marshal(c.extra); err == nil {
			fields = fields.With().RawJSON("extra", observability.RedactJSON(b)).Logger()
		}
	}
	fields.Debug().Msg("chat_completion_ok")
}

func chatCompletionLogEvent(
	log *zerolog.Logger,
	comp *sdk.ChatCompletion,
	params sdk.ChatCompletionNewParams,
	tools []llm.ToolSchema,
	msgs []llm.Message,
	dur time.Duration,
) zerolog.Context {
	fields := log.With().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Int("messages", len(msgs))
	fields = fields.Int("prompt_tokens", int(comp.Usage.PromptTokens)).
		Int("completion_tokens", int(comp.Usage.CompletionTokens)).
		Int("total_tokens", int(comp.Usage.TotalTokens))
	return appendChatUsageDetails(fields, comp.Usage)
}

func appendChatUsageDetails(fields zerolog.Context, usage sdk.CompletionUsage) zerolog.Context {
	var usageMap map[string]any
	if b, err := json.Marshal(usage); err == nil {
		_ = json.Unmarshal(b, &usageMap)
	}
	for _, group := range []string{"prompt_tokens_details", "completion_tokens_details"} {
		if values, ok := usageMap[group].(map[string]any); ok {
			for k, val := range values {
				if num, ok := val.(float64); ok {
					fields = fields.Int(group+"_"+k, int(num))
				}
			}
		}
	}
	return fields
}

func (c *Client) messageFromChatCompletion(comp *sdk.ChatCompletion, model string) llm.Message {
	if len(comp.Choices) == 0 {
		return llm.Message{}
	}
	msg := comp.Choices[0].Message
	out := llm.Message{Role: "assistant", Content: msg.Content}
	if c.isSelfHosted() {
		out.Content = stripTaggedThoughtContent(out.Content)
	}
	gemini := isGemini3Model(model)
	for _, tc := range msg.ToolCalls {
		if call, ok := chatCompletionToolCall(tc, gemini); ok {
			out.ToolCalls = append(out.ToolCalls, call)
		}
	}
	return out
}

func chatCompletionToolCall(tc sdk.ChatCompletionMessageToolCallUnion, gemini bool) (llm.ToolCall, bool) {
	switch v := tc.AsAny().(type) {
	case sdk.ChatCompletionMessageFunctionToolCall:
		sig := ""
		if gemini {
			sig = extractThoughtSignature(v.RawJSON())
		}
		return llm.ToolCall{Name: v.Function.Name, Args: json.RawMessage(v.Function.Arguments), ID: v.ID, ThoughtSignature: sig}, true
	case sdk.ChatCompletionMessageCustomToolCall:
		return llm.ToolCall{Name: v.Custom.Name, Args: json.RawMessage(v.Custom.Input), ID: v.ID}, true
	default:
		return llm.ToolCall{}, false
	}
}

func (c *Client) recordChatCompletionUsage(
	ctx context.Context,
	span trace.Span,
	usage sdk.CompletionUsage,
	model string,
	msgs []llm.Message,
	content string,
) {
	promptTokens := int(usage.PromptTokens)
	completionTokens := int(usage.CompletionTokens)
	totalTokens := int(usage.TotalTokens)
	if c.isSelfHosted() && !hasChatCompletionUsage(usage) {
		promptTokens = c.tokenizeCount(ctx, buildPromptText(msgs))
		completionTokens = c.tokenizeCount(ctx, content)
		totalTokens = promptTokens + completionTokens
	}
	llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
	llm.RecordTokenMetricsFromContext(ctx, model, promptTokens, completionTokens)
}

func hasChatCompletionUsage(usage sdk.CompletionUsage) bool {
	return usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0
}

// ChatStream implements streaming chat completions using OpenAI's streaming API.
func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	if imgOpts, ok := llm.ImagePromptFromContext(ctx); ok {
		return c.streamImageChatResult(ctx, msgs, model, imgOpts, h)
	}
	tools = c.requestTools(tools)
	if strings.EqualFold(c.api, "responses") {
		return c.chatStreamResponses(ctx, msgs, tools, model, h)
	}
	if c.isSelfHosted() {
		return c.chatStreamSSEFallback(ctx, msgs, tools, model, h)
	}
	return c.chatCompletionStream(ctx, msgs, tools, model, h)
}

func (c *Client) streamImageChatResult(ctx context.Context, msgs []llm.Message, model string, imgOpts llm.ImagePromptOptions, h llm.StreamHandler) error {
	msg, err := c.chatWithImageGeneration(ctx, msgs, model, imgOpts)
	if err != nil {
		return err
	}
	if h == nil {
		return nil
	}
	if strings.TrimSpace(msg.Content) != "" {
		h.OnDelta(msg.Content)
	}
	for _, img := range msg.Images {
		h.OnImage(img)
	}
	return nil
}

func (c *Client) chatCompletionStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	ctx, span := llm.StartRequestSpan(ctx, "OpenAI ChatStream", firstNonEmpty(model, c.model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	params := c.chatCompletionStreamParams(msgs, tools, model)

	start := time.Now()
	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params)
	defer func() {
		_ = stream.Close()
	}()

	state := newChatCompletionStreamState(string(params.Model))

	for stream.Next() {
		state.handleChunk(stream.Current(), h, log)
	}

	err := stream.Err()
	dur := time.Since(start)
	base := state.logger(log, string(params.Model), len(tools), dur)
	if err != nil {
		base.Error().Err(err).Msg("chat_stream_error")
		span.RecordError(err)
	} else {
		if c.isSelfHosted() {
			state.promptTokens = c.tokenizeCount(ctx, buildPromptText(msgs))
			state.completionTokens = c.tokenizeCount(ctx, state.assistantContent())
			state.totalTokens = state.promptTokens + state.completionTokens
		}
		state.recordSuccess(ctx, span, string(params.Model))
		base.Debug().Msg("chat_stream_ok")
	}
	return err
}

func (c *Client) chatCompletionStreamParams(msgs []llm.Message, tools []llm.ToolSchema, model string) sdk.ChatCompletionNewParams {
	params := sdk.ChatCompletionNewParams{Model: sdk.ChatModel(firstNonEmpty(model, c.model))}
	params.Messages = AdaptMessages(string(params.Model), c.chatCompletionMessages(msgs))
	actualTools := configureChatCompletionTools(&params, tools, c.isSelfHosted())
	if len(c.extra) > 0 {
		extra := c.extra
		if !actualTools {
			extra = make(map[string]any, len(c.extra))
			maps.Copy(extra, c.extra)
			delete(extra, "parallel_tool_calls")
		}
		params.SetExtraFields(sanitizeExtraFields(extra))
	}
	if !c.isSelfHosted() {
		params.StreamOptions.IncludeUsage = sdk.Bool(true)
	}
	return params
}

type chatCompletionStreamState struct {
	toolCalls               map[int]*llm.ToolCall
	toolCallsFlushed        bool
	promptTokens            int
	completionTokens        int
	totalTokens             int
	promptDetails           map[string]int
	completionDetails       map[string]int
	assistantContentBuilder strings.Builder
	gemini                  bool
}

func newChatCompletionStreamState(model string) *chatCompletionStreamState {
	return &chatCompletionStreamState{
		toolCalls:         make(map[int]*llm.ToolCall),
		promptDetails:     make(map[string]int),
		completionDetails: make(map[string]int),
		gemini:            isGemini3Model(model),
	}
}

func (s *chatCompletionStreamState) handleChunk(chunk sdk.ChatCompletionChunk, h llm.StreamHandler, log *zerolog.Logger) {
	if len(chunk.Choices) == 0 {
		s.recordUsage(chunk)
		return
	}
	choice := chunk.Choices[0]
	s.emitContent(choice.Delta.Content, h)
	for _, tc := range choice.Delta.ToolCalls {
		s.accumulateToolCall(tc)
	}
	if choice.FinishReason != "" && !s.toolCallsFlushed {
		s.flushToolCalls(h, log)
		s.toolCallsFlushed = true
	}
}

func (s *chatCompletionStreamState) emitContent(content string, h llm.StreamHandler) {
	if content == "" {
		return
	}
	h.OnDelta(content)
	s.assistantContentBuilder.WriteString(content)
}

func (s *chatCompletionStreamState) recordUsage(chunk sdk.ChatCompletionChunk) {
	if !chunk.JSON.Usage.Valid() || chunk.JSON.Usage.Raw() == "null" {
		return
	}
	s.promptTokens = int(chunk.Usage.PromptTokens)
	s.completionTokens = int(chunk.Usage.CompletionTokens)
	s.totalTokens = int(chunk.Usage.TotalTokens)
	s.recordUsageDetails(chunk.JSON.Usage.Raw())
}

func (s *chatCompletionStreamState) recordUsageDetails(raw string) {
	if raw == "" || raw == "null" {
		return
	}
	var usageMap map[string]any
	if err := json.Unmarshal([]byte(raw), &usageMap); err != nil {
		return
	}
	copyNumericUsageDetails(s.promptDetails, usageMap["prompt_tokens_details"])
	copyNumericUsageDetails(s.completionDetails, usageMap["completion_tokens_details"])
}

func copyNumericUsageDetails(dst map[string]int, raw any) {
	values, ok := raw.(map[string]any)
	if !ok {
		return
	}
	for k, val := range values {
		if num, ok := val.(float64); ok {
			dst[k] = int(num)
		}
	}
}

func (s *chatCompletionStreamState) accumulateToolCall(tc sdk.ChatCompletionChunkChoiceDeltaToolCall) {
	idx := int(tc.Index)
	if s.toolCalls[idx] == nil {
		s.toolCalls[idx] = &llm.ToolCall{ID: tc.ID}
	}
	call := s.toolCalls[idx]
	if tc.Function.Name != "" {
		call.Name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		call.Args = appendRawMessage(call.Args, tc.Function.Arguments)
	}
	if s.gemini && call.ThoughtSignature == "" {
		call.ThoughtSignature = extractThoughtSignature(tc.RawJSON())
	}
}

func appendRawMessage(existing json.RawMessage, next string) json.RawMessage {
	if existing == nil {
		return json.RawMessage(next)
	}
	return json.RawMessage(string(existing) + next)
}

func (s *chatCompletionStreamState) flushToolCalls(h llm.StreamHandler, log *zerolog.Logger) {
	for _, tc := range s.toolCalls {
		if tc != nil && tc.Name != "" && !isEmptyArgsBytes(tc.Args) {
			h.OnToolCall(*tc)
		} else if tc != nil && tc.Name != "" {
			log.Warn().Str("tool", tc.Name).Str("id", tc.ID).Msg("skipping tool call with empty arguments in stream")
		}
	}
}

func (s *chatCompletionStreamState) logger(log *zerolog.Logger, model string, toolCount int, dur time.Duration) zerolog.Logger {
	builder := log.With().
		Str("model", model).
		Int("tools", toolCount).
		Dur("duration", dur).
		Int("prompt_tokens", s.promptTokens).
		Int("completion_tokens", s.completionTokens).
		Int("total_tokens", s.totalTokens)
	for k, v := range s.promptDetails {
		builder = builder.Int("prompt_tokens_details_"+k, v)
	}
	for k, v := range s.completionDetails {
		builder = builder.Int("completion_tokens_details_"+k, v)
	}
	return builder.Logger()
}

func (s *chatCompletionStreamState) assistantContent() string {
	return s.assistantContentBuilder.String()
}

func (s *chatCompletionStreamState) recordSuccess(ctx context.Context, span trace.Span, model string) {
	llm.RecordTokenAttributes(span, s.promptTokens, s.completionTokens, s.totalTokens)
	llm.LogRedactedResponse(ctx, map[string]int{
		"prompt_tokens":     s.promptTokens,
		"completion_tokens": s.completionTokens,
		"total_tokens":      s.totalTokens,
	})
	if s.promptTokens > 0 || s.completionTokens > 0 {
		llm.RecordTokenMetricsFromContext(ctx, model, s.promptTokens, s.completionTokens)
	}
}
