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
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/observability"
)

const defaultMaxTokens int64 = 1024

type Client struct {
	sdk       anthropic.Client
	model     string
	maxTokens int64
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

	return &Client{
		sdk:       anthropic.NewClient(opts...),
		model:     model,
		maxTokens: defaultMaxTokens,
	}
}

func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	sys, converted, err := adaptMessages(msgs)
	if err != nil {
		return llm.Message{}, err
	}

	// NOTE: do not enforce model-specific token limits here; summarization and
	// context budgeting are handled centrally in the agent engine using llm.ContextSize
	// and per-model config. This avoids duplicating hard-coded thresholds here.
	toolDefs, err := adaptTools(tools)
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

	log.Debug().
		Str("model", string(params.Model)).
		Int("tools", len(tools)).
		Dur("duration", dur).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Msg("anthropic_chat_ok")

	return out, nil
}

func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	sys, converted, err := adaptMessages(msgs)
	if err != nil {
		return err
	}

	// NOTE: do not enforce model-specific token limits here; summarization and
	// context budgeting are handled centrally in the agent engine using llm.ContextSize
	// and per-model config. This avoids duplicating hard-coded thresholds here.
	toolDefs, err := adaptTools(tools)
	if err != nil {
		return err
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.pickModel(model)),
		Messages:  converted,
		System:    sys,
		Tools:     toolDefs,
		MaxTokens: c.maxTokens,
	}

	ctx, span := llm.StartRequestSpan(ctx, "Anthropic ChatStream", string(params.Model), len(tools), len(msgs))
	defer span.End()
	llm.LogRedactedPrompt(ctx, msgs)
	log := observability.LoggerWithTrace(ctx)

	start := time.Now()
	log.Debug().Str("model", string(params.Model)).Int("tools", len(tools)).Int("msgs", len(msgs)).Msg("anthropic_stream_start")

	stream := c.sdk.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var acc anthropic.Message
	var usage anthropic.MessageDeltaUsage
	toolBuffers := map[int]*toolBuffer{}
	hasDelta := false

	for stream.Next() {
		event := stream.Current()
		// The SDK's Accumulate method has a bug where it fails to marshal
		// content blocks with empty/invalid Input JSON (e.g., tool calls with
		// no arguments). We track tool calls ourselves via toolBuffers, so
		// we can safely ignore this specific error.
		if err := acc.Accumulate(event); err != nil {
			// Log detailed error for debugging
			log.Debug().Err(err).Msg("anthropic_accumulate_error")
		}

		switch ev := event.AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			switch block := ev.ContentBlock.AsAny().(type) {
			case anthropic.ToolUseBlock:
				id := strings.TrimSpace(block.ID)
				if id == "" {
					id = fmt.Sprintf("call-%d", len(toolBuffers)+1)
				}
				// Log the initial input for debugging
				rawInput, _ := json.Marshal(block.Input)
				log.Debug().Str("id", id).Str("input", string(rawInput)).Msg("anthropic_tool_start")

				tb := &toolBuffer{name: block.Name, id: id}
				tb.appendInitial(block.Input)
				toolBuffers[int(ev.Index)] = tb
			}
		case anthropic.ContentBlockDeltaEvent:
			switch delta := ev.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if h != nil && delta.Text != "" {
					h.OnDelta(delta.Text)
					hasDelta = true
				}
			case anthropic.InputJSONDelta:
				// Log partial JSON for debugging
				log.Debug().Int("index", int(ev.Index)).Str("partial", delta.PartialJSON).Msg("anthropic_tool_delta")
				if tb := toolBuffers[int(ev.Index)]; tb != nil {
					tb.appendPartial(delta.PartialJSON)
				}
			}
		case anthropic.MessageDeltaEvent:
			usage = ev.Usage
		}
	}

	if err := stream.Err(); err != nil {
		dur := time.Since(start)
		span.RecordError(err)
		log.Error().Err(err).Str("model", string(params.Model)).Dur("duration", dur).Msg("anthropic_stream_error")
		return err
	}

	// Extract tool calls from the SDK's accumulated message.
	msg := messageFromResponse(&acc)

	// Check if any toolBuffer received streaming deltas - if so, prefer our tracking
	// because the SDK doesn't correctly accumulate partial JSON from InputJSONDelta events.
	hasStreamedDeltas := false
	for _, tb := range toolBuffers {
		if tb != nil && tb.hasDeltas {
			hasStreamedDeltas = true
			break
		}
	}

	// Emit tool calls - prefer our toolBuffers when streaming deltas were received
	if len(toolBuffers) > 0 && hasStreamedDeltas {
		// Use our own tracking since we received streaming partial JSON
		log.Debug().Int("count", len(toolBuffers)).Msg("anthropic_using_tool_buffer_for_streamed_deltas")
		indices := make([]int, 0, len(toolBuffers))
		for i := range toolBuffers {
			indices = append(indices, i)
		}
		sort.Ints(indices)
		for _, idx := range indices {
			if tb := toolBuffers[idx]; tb != nil && h != nil {
				h.OnToolCall(tb.toToolCall())
			}
		}
	} else if len(msg.ToolCalls) > 0 {
		// Use SDK's accumulated data when no streaming deltas
		for _, tc := range msg.ToolCalls {
			if h != nil {
				h.OnToolCall(tc)
			}
		}
	} else if len(toolBuffers) > 0 {
		// Fallback to our own tracking if SDK didn't capture tool calls
		log.Debug().Int("count", len(toolBuffers)).Msg("anthropic_using_tool_buffer_fallback")
		indices := make([]int, 0, len(toolBuffers))
		for i := range toolBuffers {
			indices = append(indices, i)
		}
		sort.Ints(indices)
		for _, idx := range indices {
			if tb := toolBuffers[idx]; tb != nil && h != nil {
				h.OnToolCall(tb.toToolCall())
			}
		}
	}
	if !hasDelta && h != nil && msg.Content != "" {
		h.OnDelta(msg.Content)
	}

	promptTokens := usagePromptTokens(usage.CacheCreationInputTokens, usage.CacheReadInputTokens, usage.InputTokens)
	completionTokens := int(usage.OutputTokens)
	totalTokens := promptTokens + completionTokens
	llm.RecordTokenAttributes(span, promptTokens, completionTokens, totalTokens)
	llm.RecordTokenMetricsFromContext(ctx, string(params.Model), promptTokens, completionTokens)
	llm.LogRedactedResponse(ctx, acc)

	dur := time.Since(start)
	log.Debug().
		Str("model", string(params.Model)).
		Int("tools", len(tools)).
		Dur("duration", dur).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Msg("anthropic_stream_ok")

	return nil
}

func (c *Client) pickModel(model string) string {
	if m := strings.TrimSpace(model); m != "" {
		return m
	}
	return c.model
}

func adaptTools(tools []llm.ToolSchema) ([]anthropic.ToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return nil, fmt.Errorf("anthropic provider: tool name required")
		}
		schema := anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
		}
		extras := map[string]any{}
		for k, v := range t.Parameters {
			extras[k] = v
		}
		if props, ok := extras["properties"]; ok {
			schema.Properties = props
			delete(extras, "properties")
		}
		if req, ok := extras["required"]; ok {
			delete(extras, "required")
			switch v := req.(type) {
			case []string:
				schema.Required = v
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						schema.Required = append(schema.Required, s)
					}
				}
			}
		}
		delete(extras, "type")
		if len(extras) > 0 {
			schema.ExtraFields = extras
		}

		param := anthropic.ToolParam{
			Name:        name,
			InputSchema: schema,
		}
		if desc := strings.TrimSpace(t.Description); desc != "" {
			param.Description = anthropic.String(desc)
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &param})
	}
	return out, nil
}

func adaptMessages(msgs []llm.Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam, error) {
	if len(msgs) == 0 {
		return nil, nil, fmt.Errorf("messages required")
	}
	var system []anthropic.TextBlockParam
	out := make([]anthropic.MessageParam, 0, len(msgs))
	toolResultCount := 0

	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				system = append(system, anthropic.TextBlockParam{Text: m.Content})
			}
		case "user":
			blocks := []anthropic.ContentBlockParamUnion{}
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewUserMessage(blocks...))
			}
		case "assistant":
			blocks := []anthropic.ContentBlockParamUnion{}
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for i, tc := range m.ToolCalls {
				id := strings.TrimSpace(tc.ID)
				if id == "" {
					id = fmt.Sprintf("call-%d", i+1)
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(id, decodeArgs(tc.Args), tc.Name))
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewAssistantMessage(blocks...))
			}
		case "tool":
			id := strings.TrimSpace(m.ToolID)
			if id == "" {
				toolResultCount++
				id = fmt.Sprintf("tool-result-%d", toolResultCount)
			}
			out = append(out, anthropic.NewUserMessage(anthropic.NewToolResultBlock(id, m.Content, false)))
		default:
			return nil, nil, fmt.Errorf("unsupported role for anthropic provider: %s", m.Role)
		}
	}
	return system, out, nil
}

func decodeArgs(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		return m
	}
	// If we can't unmarshal to a map, return empty object
	// Anthropic requires tool_use.input to be a valid dictionary
	return map[string]any{}
}

func messageFromResponse(resp *anthropic.Message) llm.Message {
	if resp == nil {
		return llm.Message{}
	}
	var sb strings.Builder
	var calls []llm.ToolCall
	callIdx := 0

	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			sb.WriteString(v.Text)
		case anthropic.ToolUseBlock:
			callIdx++
			id := strings.TrimSpace(v.ID)
			if id == "" {
				id = fmt.Sprintf("call-%d", callIdx)
			}
			args := v.Input
			if len(args) == 0 {
				if b, err := json.Marshal(v.Input); err == nil {
					args = b
				}
			}
			calls = append(calls, llm.ToolCall{
				Name: v.Name,
				Args: args,
				ID:   id,
			})
		}
	}

	return llm.Message{
		Role:    "assistant",
		Content: sb.String(),
		ToolCalls: func() []llm.ToolCall {
			if len(calls) == 0 {
				return nil
			}
			return calls
		}(),
	}
}

func usagePromptTokens(cacheCreation int64, cacheRead int64, input int64) int {
	return int(cacheCreation + cacheRead + input)
}

type toolBuffer struct {
	name        string
	id          string
	buf         strings.Builder
	hasDeltas   bool
	initialJSON string
}

func (tb *toolBuffer) appendInitial(raw json.RawMessage) {
	// Store the initial JSON string. We might need to "re-open" it if deltas come.
	if len(raw) == 0 {
		// Anthropic requires tool_use.input to be a dictionary; treat empty as {} so we can append deltas safely.
		raw = json.RawMessage("{}")
	}
	tb.initialJSON = string(raw)
	tb.buf.WriteString(tb.initialJSON)
}

func (tb *toolBuffer) appendPartial(partial string) {
	if partial == "" {
		return
	}
	// If this is the first delta, we need to prepare the buffer.
	// The initial input from content_block_start is typically an empty object "{}"
	// which is just a placeholder. When streaming deltas arrive, they contain
	// the actual JSON content that should replace (not extend) the initial empty object.
	if !tb.hasDeltas {
		// Clear the buffer and start fresh with the incoming partial JSON
		tb.buf.Reset()
		tb.hasDeltas = true
	}
	tb.buf.WriteString(partial)
}

func (tb *toolBuffer) toToolCall() llm.ToolCall {
	args := tb.buf.String()
	// If we had deltas, we likely stripped the closing brace and need to restore it.
	// But only if the deltas didn't already close it (unlikely for partials, but possible).
	// Actually, simpler: if it doesn't end in '}', add it.
	if tb.hasDeltas && !strings.HasSuffix(strings.TrimSpace(args), "}") {
		args += "}"
	}

	// Ensure we always have valid JSON - default to empty object if buffer is empty or invalid
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		trimmed = "{}"
	} else {
		if !strings.HasPrefix(trimmed, "{") {
			trimmed = "{" + trimmed
		}
		if !strings.HasSuffix(trimmed, "}") {
			trimmed += "}"
		}
	}
	if !json.Valid([]byte(trimmed)) {
		trimmed = "{}"
	}

	args = trimmed
	return llm.ToolCall{
		Name: tb.name,
		Args: json.RawMessage(args),
		ID:   tb.id,
	}
}

// Tokenizer returns a MessagesTokenizer for accurate preflight token counting
// using the Anthropic /v1/messages/count_tokens endpoint.
func (c *Client) Tokenizer(cache *llm.TokenCache) llm.Tokenizer {
	return NewMessagesTokenizer(c.sdk, c.model, cache)
}

// SupportsTokenization returns true as Anthropic always supports
// the count_tokens endpoint for preflight token counting.
func (c *Client) SupportsTokenization() bool {
	return true
}
