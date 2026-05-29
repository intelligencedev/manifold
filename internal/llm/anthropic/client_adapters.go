package anthropic

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"manifold/internal/config"
	"manifold/internal/llm"
)

func (c *Client) pickModel(model string) string {
	if m := strings.TrimSpace(model); m != "" {
		return m
	}
	return c.model
}

func shouldIncludeThoughtSummaries(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if idx := strings.LastIndex(m, "/"); idx != -1 {
		m = m[idx+1:]
	}
	// Extended thinking is not available on all Claude models. Be conservative to
	// avoid 400 errors from older/non-thinking models.
	supports := []string{
		"claude-sonnet-4-5",
		"claude-haiku-4-5",
		"claude-opus-4-5",
	}
	for _, s := range supports {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

func adaptTools(tools []llm.ToolSchema, cacheCfg config.AnthropicPromptCacheConfig) ([]anthropic.ToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	cacheTools := cacheCfg.Enabled && cacheCfg.CacheTools
	cacheControl := anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL5m}
	for _, t := range tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return nil, fmt.Errorf("anthropic provider: tool name required")
		}
		schema := anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
		}
		extras := map[string]any{}
		maps.Copy(extras, t.Parameters)
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
	if cacheTools {
		if last := out[len(out)-1].OfTool; last != nil {
			last.CacheControl = cacheControl
		}
	}
	return out, nil
}

func adaptMessages(msgs []llm.Message, cacheCfg config.AnthropicPromptCacheConfig) ([]anthropic.TextBlockParam, []anthropic.MessageParam, error) {
	if len(msgs) == 0 {
		return nil, nil, fmt.Errorf("messages required")
	}
	var system []anthropic.TextBlockParam
	out := make([]anthropic.MessageParam, 0, len(msgs))
	toolResultCount := 0
	cacheSystem := cacheCfg.Enabled && cacheCfg.CacheSystem
	cacheMessages := cacheCfg.Enabled && cacheCfg.CacheMessages
	cacheControl := anthropic.CacheControlEphemeralParam{TTL: anthropic.CacheControlEphemeralTTLTTL5m}
	var lastMessageText *anthropic.TextBlockParam
	newTextBlock := func(text string) anthropic.ContentBlockParamUnion {
		block := &anthropic.TextBlockParam{Text: text}
		lastMessageText = block
		return anthropic.ContentBlockParamUnion{OfText: block}
	}

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
				blocks = append(blocks, newTextBlock(m.Content))
			}
			if len(blocks) > 0 {
				out = append(out, anthropic.NewUserMessage(blocks...))
			}
		case "assistant":
			blocks := []anthropic.ContentBlockParamUnion{}
			// Prepend thinking blocks if present. Anthropic requires assistant messages
			// to start with thinking blocks when extended thinking is enabled.
			if m.ThoughtSignature != "" {
				var savedThinking []thinkingData
				if err := json.Unmarshal([]byte(m.ThoughtSignature), &savedThinking); err == nil {
					for _, td := range savedThinking {
						blocks = append(blocks, anthropic.NewThinkingBlock(td.Signature, td.Thinking))
					}
				}
			}
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, newTextBlock(m.Content))
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
	if cacheSystem && len(system) > 0 {
		system[len(system)-1].CacheControl = cacheControl
	}
	if cacheMessages && lastMessageText != nil {
		lastMessageText.CacheControl = cacheControl
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
	var thinkingBlocks []thinkingData
	callIdx := 0

	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.ThinkingBlock:
			thinkingBlocks = append(thinkingBlocks, thinkingData{
				Signature: v.Signature,
				Thinking:  v.Thinking,
			})
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

	// Encode thinking blocks as JSON in ThoughtSignature for multi-turn preservation.
	var thoughtSig string
	if len(thinkingBlocks) > 0 {
		if encoded, err := json.Marshal(thinkingBlocks); err == nil {
			thoughtSig = string(encoded)
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
		ThoughtSignature: thoughtSig,
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
	if !tb.hasDeltas {
		// Clear the buffer and start fresh with the incoming partial JSON
		tb.buf.Reset()
		tb.hasDeltas = true
	}
	tb.buf.WriteString(partial)
}

func (tb *toolBuffer) toToolCall() llm.ToolCall {
	args := tb.buf.String()
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
