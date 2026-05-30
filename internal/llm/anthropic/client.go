package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

const defaultMaxTokens int64 = 4096

// thinkingData stores Anthropic thinking block information for multi-turn conversations.
// When extended thinking is enabled, Anthropic requires assistant messages to include
// thinking blocks from previous turns.
type thinkingData struct {
	Signature string `json:"signature"`
	Thinking  string `json:"thinking"`
}

type Client struct {
	sdk       anthropic.Client
	model     string
	maxTokens int64
	cacheCfg  config.AnthropicPromptCacheConfig
	extra     map[string]any
}

type ImageAttachment struct {
	MimeType   string
	Base64Data string
}

func New(cfg config.AnthropicConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	opts := []option.RequestOption{
		option.WithAPIKey(strings.TrimSpace(cfg.APIKey)),
		option.WithHTTPClient(httpClient),
	}
	if base := strings.TrimSpace(cfg.BaseURL); base != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimSuffix(base, "/")))
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = string(anthropic.ModelClaude3_7SonnetLatest)
	}

	cacheCfg := cfg.PromptCache
	if cacheCfg.Enabled && !cacheCfg.CacheSystem && !cacheCfg.CacheTools && !cacheCfg.CacheMessages {
		// Sensible defaults when the feature is enabled but no scope is specified.
		cacheCfg.CacheSystem = true
		cacheCfg.CacheTools = true
	}

	maxTokens := defaultMaxTokens
	if cfg.MaxTokens > 0 {
		maxTokens = cfg.MaxTokens
	}

	return &Client{
		sdk:       anthropic.NewClient(opts...),
		model:     model,
		maxTokens: maxTokens,
		cacheCfg:  cacheCfg,
		extra:     llm.NormalizeExtraParams(cfg.ExtraParams),
	}
}

func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	sys, converted, err := adaptMessages(msgs, c.cacheCfg)
	if err != nil {
		return llm.Message{}, err
	}

	// NOTE: do not enforce model-specific token limits here; summarization and
	// context budgeting are handled centrally in the agent engine using llm.ContextSize
	// and per-model config. This avoids duplicating hard-coded thresholds here.
	toolDefs, err := adaptTools(tools, c.cacheCfg)
	if err != nil {
		return llm.Message{}, err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.pickModel(model)),
		Messages:  converted,
		System:    sys,
		Tools:     toolDefs,
		MaxTokens: c.maxTokens,
	}
	if shouldIncludeThoughtSummaries(string(params.Model)) {
		// Enable extended thinking so we can stream thinking deltas and surface them
		// as thought summaries in the UI.
		//
		// Anthropic enforces budget_tokens >= 1024 AND max_tokens > budget_tokens.
		const thinkingBudget int64 = 1024
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(thinkingBudget)
		if params.MaxTokens <= thinkingBudget {
			params.MaxTokens = thinkingBudget + 1024
		}
	}
	extra := llm.NormalizeExtraParams(c.extra)
	if v, found, ok := llm.PopIntExtraParam(extra, "max_tokens", "maxTokens"); found {
		if ok {
			params.MaxTokens = v
		} else {
			log.Warn().Msg("anthropic_invalid_extra_max_tokens")
		}
	}
	if len(extra) > 0 {
		params.SetExtraFields(extra)
	}

	ctx, span := llm.StartRequestSpan(ctx, "Anthropic Chat", string(params.Model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	log := observability.LoggerWithTrace(ctx)

	start := time.Now()
	resp, err := c.sdk.Messages.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("anthropic_chat_error")
		return llm.Message{}, err
	}

	llm.LogRedactedResponse(ctx, resp)

	out := messageFromResponse(resp)

	promptTokens := usagePromptTokens(resp.Usage.CacheCreationInputTokens, resp.Usage.CacheReadInputTokens, resp.Usage.InputTokens)
	completionTokens := int(resp.Usage.OutputTokens)
	totalTokens := promptTokens + completionTokens

	llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
	llm.RecordTokenMetricsFromContext(ctx, string(params.Model), promptTokens, completionTokens)

	stopReason := string(resp.StopReason)
	if stopReason == "" {
		stopReason = "unknown"
	}

	log.Debug().
		Str("model", string(params.Model)).
		Int("tools", len(tools)).
		Dur("duration", dur).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Str("stop_reason", stopReason).
		Str("stop_sequence", resp.StopSequence).
		Msg("anthropic_chat_ok")

	return out, nil
}

func (c *Client) ChatWithImageAttachments(ctx context.Context, msgs []llm.Message, images []ImageAttachment, tools []llm.ToolSchema, model string) (llm.Message, error) {
	sys, converted, err := adaptMessages(msgs, c.cacheCfg)
	if err != nil {
		return llm.Message{}, err
	}
	toolDefs, err := adaptTools(tools, c.cacheCfg)
	if err != nil {
		return llm.Message{}, err
	}
	converted = appendImageBlocksToMessages(converted, images)
	params := c.newMessageParams(model, converted, sys, toolDefs)

	ctx, span := llm.StartRequestSpan(ctx, "Anthropic ChatWithImageAttachments", string(params.Model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	logger := observability.LoggerWithTrace(ctx)

	result, err := c.runImageChatStream(ctx, params)
	if err != nil {
		span.RecordError(err)
		logger.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", result.duration).Msg("anthropic_chat_with_images_error")
		return llm.Message{}, err
	}
	out := imageChatMessageFromResult(result)
	llm.LogRedactedResponse(ctx, result.message)
	recordAnthropicUsage(ctx, span, string(params.Model), result.usage)
	logAnthropicImageChatOK(logger, string(params.Model), len(tools), result)
	return out, nil
}

func appendImageBlocksToMessages(converted []anthropic.MessageParam, images []ImageAttachment) []anthropic.MessageParam {
	imageBlocks := imageContentBlocks(images)
	if len(imageBlocks) == 0 {
		return converted
	}
	lastUserIdx := -1
	for i := len(converted) - 1; i >= 0; i-- {
		if converted[i].Role == anthropic.MessageParamRoleUser {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx >= 0 {
		converted[lastUserIdx].Content = append(converted[lastUserIdx].Content, imageBlocks...)
		return converted
	}
	return append(converted, anthropic.NewUserMessage(imageBlocks...))
}

func imageContentBlocks(images []ImageAttachment) []anthropic.ContentBlockParamUnion {
	imageBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(images))
	for _, img := range images {
		mime := strings.ToLower(strings.TrimSpace(img.MimeType))
		if mime == "image/jpg" {
			mime = "image/jpeg"
		}
		if mime == "" || strings.TrimSpace(img.Base64Data) == "" {
			continue
		}
		imageBlocks = append(imageBlocks, anthropic.NewImageBlockBase64(mime, img.Base64Data))
	}
	return imageBlocks
}

func (c *Client) newMessageParams(
	model string,
	converted []anthropic.MessageParam,
	sys []anthropic.TextBlockParam,
	toolDefs []anthropic.ToolUnionParam,
) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.pickModel(model)),
		Messages:  converted,
		System:    sys,
		Tools:     toolDefs,
		MaxTokens: c.maxTokens,
	}
	if shouldIncludeThoughtSummaries(string(params.Model)) {
		const thinkingBudget int64 = 1024
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(thinkingBudget)
		if params.MaxTokens <= thinkingBudget {
			params.MaxTokens = thinkingBudget + 1024
		}
	}
	extra := llm.NormalizeExtraParams(c.extra)
	if v, found, ok := llm.PopIntExtraParam(extra, "max_tokens", "maxTokens"); found {
		if ok {
			params.MaxTokens = v
		} else {
			log.Warn().Msg("anthropic_invalid_extra_max_tokens")
		}
	}
	if len(extra) > 0 {
		params.SetExtraFields(extra)
	}
	return params
}

type imageChatStreamResult struct {
	message     anthropic.Message
	usage       anthropic.MessageDeltaUsage
	toolBuffers map[int]*toolBuffer
	text        string
	duration    time.Duration
}

func (c *Client) runImageChatStream(ctx context.Context, params anthropic.MessageNewParams) (imageChatStreamResult, error) {
	start := time.Now()
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	result := imageChatStreamResult{toolBuffers: map[int]*toolBuffer{}}
	textBuf := strings.Builder{}
	logger := observability.LoggerWithTrace(ctx)

	for stream.Next() {
		event := stream.Current()
		if err := result.message.Accumulate(event); err != nil {
			logger.Debug().Err(err).Msg("anthropic_accumulate_error")
		}
		accumulateImageChatEvent(event, result.toolBuffers, &textBuf, &result.usage)
	}
	result.duration = time.Since(start)
	if err := stream.Err(); err != nil {
		return result, err
	}
	result.text = textBuf.String()
	return result, nil
}

func accumulateImageChatEvent(
	event anthropic.MessageStreamEventUnion,
	toolBuffers map[int]*toolBuffer,
	textBuf *strings.Builder,
	usage *anthropic.MessageDeltaUsage,
) {
	switch ev := event.AsAny().(type) {
	case anthropic.ContentBlockStartEvent:
		if block, ok := ev.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
			id := strings.TrimSpace(block.ID)
			if id == "" {
				id = fmt.Sprintf("call-%d", len(toolBuffers)+1)
			}
			tb := &toolBuffer{name: block.Name, id: id}
			tb.appendInitial(block.Input)
			toolBuffers[int(ev.Index)] = tb
		}
	case anthropic.ContentBlockDeltaEvent:
		accumulateImageChatDelta(ev, toolBuffers, textBuf)
	case anthropic.MessageDeltaEvent:
		*usage = ev.Usage
	}
}

func accumulateImageChatDelta(
	ev anthropic.ContentBlockDeltaEvent,
	toolBuffers map[int]*toolBuffer,
	textBuf *strings.Builder,
) {
	switch delta := ev.Delta.AsAny().(type) {
	case anthropic.TextDelta:
		textBuf.WriteString(delta.Text)
	case anthropic.InputJSONDelta:
		if tb := toolBuffers[int(ev.Index)]; tb != nil {
			tb.appendPartial(delta.PartialJSON)
		}
	}
}

func imageChatMessageFromResult(result imageChatStreamResult) llm.Message {
	out := messageFromResponse(&result.message)
	if strings.TrimSpace(out.Content) == "" && result.text != "" {
		out.Content = result.text
	}
	if calls := streamedToolCalls(result.toolBuffers); len(calls) > 0 {
		out.ToolCalls = calls
	}
	return out
}

func streamedToolCalls(toolBuffers map[int]*toolBuffer) []llm.ToolCall {
	if !hasStreamedToolDeltas(toolBuffers) {
		return nil
	}
	indices := make([]int, 0, len(toolBuffers))
	for idx := range toolBuffers {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	calls := make([]llm.ToolCall, 0, len(indices))
	for _, idx := range indices {
		if tb := toolBuffers[idx]; tb != nil {
			calls = append(calls, tb.toToolCall())
		}
	}
	return calls
}

func hasStreamedToolDeltas(toolBuffers map[int]*toolBuffer) bool {
	for _, tb := range toolBuffers {
		if tb != nil && tb.hasDeltas {
			return true
		}
	}
	return false
}

func recordAnthropicUsage(ctx context.Context, span trace.Span, model string, usage anthropic.MessageDeltaUsage) {
	promptTokens := usagePromptTokens(usage.CacheCreationInputTokens, usage.CacheReadInputTokens, usage.InputTokens)
	completionTokens := int(usage.OutputTokens)
	totalTokens := promptTokens + completionTokens
	llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
	llm.RecordTokenMetricsFromContext(ctx, model, promptTokens, completionTokens)
}

func logAnthropicImageChatOK(logger *zerolog.Logger, model string, toolCount int, result imageChatStreamResult) {
	promptTokens := usagePromptTokens(result.usage.CacheCreationInputTokens, result.usage.CacheReadInputTokens, result.usage.InputTokens)
	completionTokens := int(result.usage.OutputTokens)
	totalTokens := promptTokens + completionTokens
	stopReason := string(result.message.StopReason)
	if stopReason == "" {
		stopReason = "unknown"
	}
	logger.Debug().
		Str("model", model).
		Int("tools", toolCount).
		Dur("duration", result.duration).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Str("stop_reason", stopReason).
		Str("stop_sequence", result.message.StopSequence).
		Msg("anthropic_chat_with_images_ok")
}

func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	sys, converted, err := adaptMessages(msgs, c.cacheCfg)
	if err != nil {
		return err
	}
	toolDefs, err := adaptTools(tools, c.cacheCfg)
	if err != nil {
		return err
	}
	params := c.newMessageParams(model, converted, sys, toolDefs)
	ctx, span := llm.StartRequestSpan(ctx, "Anthropic ChatStream", string(params.Model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	logger := observability.LoggerWithTrace(ctx)
	logger.Debug().Str("model", string(params.Model)).Int("tools", len(tools)).Int("msgs", len(msgs)).Msg("anthropic_stream_start")

	result, err := c.runChatStream(ctx, params, h)
	if err != nil {
		span.RecordError(err)
		logger.Error().Err(err).Str("model", string(params.Model)).Dur("duration", result.duration).Msg("anthropic_stream_error")
		return err
	}
	msg := messageFromResponse(&result.message)
	emitChatStreamResult(h, msg, result, logger)
	recordAnthropicUsage(ctx, span, string(params.Model), result.usage)
	llm.LogRedactedResponse(ctx, result.message)
	logAnthropicStreamOK(logger, string(params.Model), len(tools), result)
	return nil
}

type chatStreamResult struct {
	message        anthropic.Message
	usage          anthropic.MessageDeltaUsage
	toolBuffers    map[int]*toolBuffer
	thinkingBlocks map[int64]*strings.Builder
	thinkingCount  int
	hasDelta       bool
	duration       time.Duration
}

func (c *Client) runChatStream(ctx context.Context, params anthropic.MessageNewParams, h llm.StreamHandler) (chatStreamResult, error) {
	start := time.Now()
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()
	result := chatStreamResult{toolBuffers: map[int]*toolBuffer{}, thinkingBlocks: map[int64]*strings.Builder{}}
	logger := observability.LoggerWithTrace(ctx)
	for stream.Next() {
		event := stream.Current()
		if err := result.message.Accumulate(event); err != nil {
			logger.Debug().Err(err).Msg("anthropic_accumulate_error")
		}
		accumulateChatStreamEvent(event, h, &result, logger)
	}
	result.duration = time.Since(start)
	return result, stream.Err()
}

func accumulateChatStreamEvent(event anthropic.MessageStreamEventUnion, h llm.StreamHandler, result *chatStreamResult, logger *zerolog.Logger) {
	switch ev := event.AsAny().(type) {
	case anthropic.ContentBlockStartEvent:
		accumulateChatStreamBlockStart(ev, h, result, logger)
	case anthropic.ContentBlockDeltaEvent:
		accumulateChatStreamBlockDelta(ev, h, result, logger)
	case anthropic.MessageDeltaEvent:
		result.usage = ev.Usage
	}
}

func accumulateChatStreamBlockStart(ev anthropic.ContentBlockStartEvent, h llm.StreamHandler, result *chatStreamResult, logger *zerolog.Logger) {
	switch block := ev.ContentBlock.AsAny().(type) {
	case anthropic.ThinkingBlock:
		if h == nil {
			return
		}
		b := &strings.Builder{}
		b.WriteString(block.Thinking)
		result.thinkingBlocks[ev.Index] = b
		result.thinkingCount++
		if b.Len() > 0 {
			h.OnThoughtSummary(b.String())
		}
	case anthropic.ToolUseBlock:
		id := strings.TrimSpace(block.ID)
		if id == "" {
			id = fmt.Sprintf("call-%d", len(result.toolBuffers)+1)
		}
		rawInput, _ := json.Marshal(block.Input)
		logger.Debug().Str("id", id).Str("input", string(rawInput)).Msg("anthropic_tool_start")
		tb := &toolBuffer{name: block.Name, id: id}
		tb.appendInitial(block.Input)
		result.toolBuffers[int(ev.Index)] = tb
	}
}

func accumulateChatStreamBlockDelta(ev anthropic.ContentBlockDeltaEvent, h llm.StreamHandler, result *chatStreamResult, logger *zerolog.Logger) {
	switch delta := ev.Delta.AsAny().(type) {
	case anthropic.TextDelta:
		if h != nil && delta.Text != "" {
			h.OnDelta(delta.Text)
			result.hasDelta = true
		}
	case anthropic.InputJSONDelta:
		logger.Debug().Int("index", int(ev.Index)).Str("partial", delta.PartialJSON).Msg("anthropic_tool_delta")
		if tb := result.toolBuffers[int(ev.Index)]; tb != nil {
			tb.appendPartial(delta.PartialJSON)
		}
	case anthropic.ThinkingDelta:
		accumulateChatStreamThinkingDelta(ev.Index, delta.Thinking, h, result)
	}
}

func accumulateChatStreamThinkingDelta(index int64, thinking string, h llm.StreamHandler, result *chatStreamResult) {
	if h == nil || thinking == "" {
		return
	}
	b := result.thinkingBlocks[index]
	if b == nil {
		b = &strings.Builder{}
		result.thinkingBlocks[index] = b
		result.thinkingCount++
	}
	b.WriteString(thinking)
	h.OnThoughtSummary(b.String())
}

func emitChatStreamResult(h llm.StreamHandler, msg llm.Message, result chatStreamResult, logger *zerolog.Logger) {
	if h == nil {
		return
	}
	emitChatStreamToolCalls(h, msg, result.toolBuffers, logger)
	if !result.hasDelta && msg.Content != "" {
		h.OnDelta(msg.Content)
	}
	emitChatStreamThoughtSignature(h, result.message, result.thinkingBlocks)
}

func emitChatStreamToolCalls(h llm.StreamHandler, msg llm.Message, toolBuffers map[int]*toolBuffer, logger *zerolog.Logger) {
	switch {
	case len(toolBuffers) > 0 && hasStreamedToolDeltas(toolBuffers):
		logger.Debug().Int("count", len(toolBuffers)).Msg("anthropic_using_tool_buffer_for_streamed_deltas")
		emitBufferedToolCalls(h, toolBuffers)
	case len(msg.ToolCalls) > 0:
		for _, tc := range msg.ToolCalls {
			h.OnToolCall(tc)
		}
	case len(toolBuffers) > 0:
		logger.Debug().Int("count", len(toolBuffers)).Msg("anthropic_using_tool_buffer_fallback")
		emitBufferedToolCalls(h, toolBuffers)
	}
}

func emitBufferedToolCalls(h llm.StreamHandler, toolBuffers map[int]*toolBuffer) {
	indices := make([]int, 0, len(toolBuffers))
	for i := range toolBuffers {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		if tb := toolBuffers[idx]; tb != nil {
			h.OnToolCall(tb.toToolCall())
		}
	}
}

func emitChatStreamThoughtSignature(h llm.StreamHandler, acc anthropic.Message, thinkingBlocks map[int64]*strings.Builder) {
	if len(thinkingBlocks) == 0 {
		return
	}
	var thinkingData []thinkingData
	for _, block := range acc.Content {
		if tb, ok := block.AsAny().(anthropic.ThinkingBlock); ok {
			thinkingData = append(thinkingData, struct {
				Signature string `json:"signature"`
				Thinking  string `json:"thinking"`
			}{Signature: tb.Signature, Thinking: tb.Thinking})
		}
	}
	if len(thinkingData) > 0 {
		if encoded, err := json.Marshal(thinkingData); err == nil {
			h.OnThoughtSignature(string(encoded))
		}
	}
}

func logAnthropicStreamOK(logger *zerolog.Logger, model string, toolCount int, result chatStreamResult) {
	promptTokens := usagePromptTokens(result.usage.CacheCreationInputTokens, result.usage.CacheReadInputTokens, result.usage.InputTokens)
	completionTokens := int(result.usage.OutputTokens)
	totalTokens := promptTokens + completionTokens
	stopReason := string(result.message.StopReason)
	if stopReason == "" {
		stopReason = "unknown"
	}
	logger.Debug().
		Str("model", model).
		Int("tools", toolCount).
		Dur("duration", result.duration).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Int("thought_summaries", result.thinkingCount).
		Str("stop_reason", stopReason).
		Str("stop_sequence", result.message.StopSequence).
		Msg("anthropic_stream_ok")
}
