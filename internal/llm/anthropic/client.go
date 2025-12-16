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
	llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)

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

	stream := c.sdk.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var acc anthropic.Message
	var usage anthropic.MessageDeltaUsage
	toolBuffers := map[int]*toolBuffer{}
	hasDelta := false

	for stream.Next() {
		event := stream.Current()
		if err := acc.Accumulate(event); err != nil {
			span.RecordError(err)
			return err
		}

		switch ev := event.AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			switch block := ev.ContentBlock.AsAny().(type) {
			case anthropic.ToolUseBlock:
				id := strings.TrimSpace(block.ID)
				if id == "" {
					id = fmt.Sprintf("call-%d", len(toolBuffers)+1)
				}
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
				if tb := toolBuffers[int(ev.Index)]; tb != nil {
					tb.appendPartial(delta.PartialJSON)
				}
			}
		case anthropic.MessageDeltaEvent:
			usage = ev.Usage
		}
	}

	if err := stream.Err(); err != nil {
		span.RecordError(err)
		log.Error().Err(err).Str("model", string(params.Model)).Msg("anthropic_stream_error")
		return err
	}

	// Emit any tool calls assembled during the stream
	msg := messageFromResponse(&acc)
	if len(toolBuffers) > 0 {
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
	} else {
		for _, tc := range msg.ToolCalls {
			if h != nil {
				h.OnToolCall(tc)
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
	llm.RecordTokenMetrics(string(params.Model), promptTokens, completionTokens)
	llm.LogRedactedResponse(ctx, acc)

	log.Debug().
		Str("model", string(params.Model)).
		Int("tools", len(tools)).
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
	var m any
	if err := json.Unmarshal(raw, &m); err == nil {
		return m
	}
	return string(raw)
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
	name string
	id   string
	buf  strings.Builder
}

func (tb *toolBuffer) appendInitial(raw json.RawMessage) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "" && trimmed != "{}" {
		tb.buf.Write(raw)
	}
}

func (tb *toolBuffer) appendPartial(partial string) {
	if partial == "" {
		return
	}
	if tb.buf.Len() == 0 {
		tb.buf.WriteString(partial)
		return
	}
	tb.buf.WriteString(partial)
}

func (tb *toolBuffer) toToolCall() llm.ToolCall {
	args := json.RawMessage(tb.buf.String())
	return llm.ToolCall{
		Name: tb.name,
		Args: args,
		ID:   tb.id,
	}
}
