package openai

import (
	"context"
	"encoding/json"

	"net/http"
	"time"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"

	"singularityio/internal/config"
	"singularityio/internal/llm"
	"singularityio/internal/observability"
)

type Client struct {
	sdk         sdk.Client
	model       string
	extra       map[string]any
	logPayloads bool
}

func New(c config.OpenAIConfig, httpClient *http.Client) *Client {
	opts := []option.RequestOption{option.WithAPIKey(c.APIKey)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}
	return &Client{sdk: sdk.NewClient(opts...), model: c.Model, extra: c.ExtraParams, logPayloads: c.LogPayloads}
}

// Chat implements llm.Provider.Chat using OpenAI Chat Completions.
func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	// messages
	params.Messages = AdaptMessages(msgs)
	// tools: include only when provided to avoid sending an empty array
	if len(tools) > 0 {
		params.Tools = AdaptSchemas(tools)
	}
	if len(c.extra) > 0 {
		params.SetExtraFields(c.extra)
	}
	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_error")
		return llm.Message{}, err
	}
	f := log.With().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Int("messages", len(msgs))
	f = f.Int("prompt_tokens", int(comp.Usage.PromptTokens)).
		Int("completion_tokens", int(comp.Usage.CompletionTokens)).
		Int("total_tokens", int(comp.Usage.TotalTokens))
	fields := f.Logger()
	if c.logPayloads && c.extra != nil && len(c.extra) > 0 {
		if b, err := json.Marshal(c.extra); err == nil {
			fields = fields.With().RawJSON("extra", observability.RedactJSON(b)).Logger()
		}
	}
	fields.Debug().Msg("chat_completion_ok")
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	msg := comp.Choices[0].Message
	out := llm.Message{Role: "assistant", Content: msg.Content}
	for _, tc := range msg.ToolCalls {
		switch v := tc.AsAny().(type) {
		case sdk.ChatCompletionMessageFunctionToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Function.Name,
				Args: json.RawMessage(v.Function.Arguments),
				ID:   v.ID,
			})
		case sdk.ChatCompletionMessageCustomToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Custom.Name,
				Args: json.RawMessage(v.Custom.Input),
				ID:   v.ID,
			})
		}
	}
	return out, nil
}

// ChatWithOptions is like Chat but allows callers to:
//   - omit tools entirely by passing a nil or empty tools slice
//   - inject provider-specific extra fields (e.g., reasoning.effort)
//     via params.WithExtraField.
func (c *Client) ChatWithOptions(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, extra map[string]any) (llm.Message, error) {
	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	params.Messages = AdaptMessages(msgs)
	if len(tools) > 0 {
		params.Tools = AdaptSchemas(tools)
	}
	if len(c.extra) > 0 || len(extra) > 0 {
		merged := make(map[string]any, len(c.extra)+len(extra))
		for k, v := range c.extra {
			merged[k] = v
		}
		for k, v := range extra {
			merged[k] = v
		}
		params.SetExtraFields(merged)
	}
	start := time.Now()
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	dur := time.Since(start)
	if err != nil {
		log.Error().Err(err).Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Msg("chat_completion_error")
		return llm.Message{}, err
	}
	f := log.With().Str("model", string(params.Model)).Int("tools", len(tools)).Dur("duration", dur).Int("messages", len(msgs))
	f = f.Int("prompt_tokens", int(comp.Usage.PromptTokens)).
		Int("completion_tokens", int(comp.Usage.CompletionTokens)).
		Int("total_tokens", int(comp.Usage.TotalTokens))
	fields := f.Logger()
	if c.logPayloads && c.extra != nil && len(c.extra) > 0 {
		if b, err := json.Marshal(c.extra); err == nil {
			fields = fields.With().RawJSON("extra", observability.RedactJSON(b)).Logger()
		}
	}
	fields.Debug().Msg("chat_completion_ok")
	if len(comp.Choices) == 0 {
		return llm.Message{}, nil
	}
	msg := comp.Choices[0].Message
	out := llm.Message{Role: "assistant", Content: msg.Content}
	for _, tc := range msg.ToolCalls {
		switch v := tc.AsAny().(type) {
		case sdk.ChatCompletionMessageFunctionToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Function.Name,
				Args: json.RawMessage(v.Function.Arguments),
				ID:   v.ID,
			})
		case sdk.ChatCompletionMessageCustomToolCall:
			out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
				Name: v.Custom.Name,
				Args: json.RawMessage(v.Custom.Input),
				ID:   v.ID,
			})
		}
	}
	return out, nil
}

// ChatStream implements streaming chat completions using OpenAI's streaming API.
func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	log := observability.LoggerWithTrace(ctx)
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	// messages
	params.Messages = AdaptMessages(msgs)
	// tools
	if len(tools) > 0 {
		params.Tools = AdaptSchemas(tools)
	}
	if len(c.extra) > 0 {
		params.SetExtraFields(c.extra)
	}
	// Ask the API to include a final usage chunk so we can log token counts
	params.StreamOptions.IncludeUsage = sdk.Bool(true)

	start := time.Now()
	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params)
	defer func() {
		_ = stream.Close()
	}()

	// Accumulate tool calls across chunks since they come incrementally
	toolCalls := make(map[int]*llm.ToolCall)
	// Track whether we've flushed tool calls to the handler
	toolCallsFlushed := false
	// Track token usage (filled from the final usage chunk if available)
	var promptTokens, completionTokens, totalTokens int

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			// Even if there are no choices, the final chunk may contain usage
			if chunk.JSON.Usage.Valid() && chunk.JSON.Usage.Raw() != "null" {
				promptTokens = int(chunk.Usage.PromptTokens)
				completionTokens = int(chunk.Usage.CompletionTokens)
				totalTokens = int(chunk.Usage.TotalTokens)
			}
			continue
		}

		delta := chunk.Choices[0].Delta

		// Handle content deltas
		if delta.Content != "" {
			h.OnDelta(delta.Content)
		}

		// Handle tool calls - accumulate across chunks
		for i, tc := range delta.ToolCalls {
			if toolCalls[i] == nil {
				toolCalls[i] = &llm.ToolCall{
					ID: tc.ID,
				}
			}

			// Accumulate function name
			if tc.Function.Name != "" {
				toolCalls[i].Name = tc.Function.Name
			}

			// Accumulate function arguments
			if tc.Function.Arguments != "" {
				if toolCalls[i].Args == nil {
					toolCalls[i].Args = json.RawMessage(tc.Function.Arguments)
				} else {
					// Append new arguments to existing ones
					existing := string(toolCalls[i].Args)
					toolCalls[i].Args = json.RawMessage(existing + tc.Function.Arguments)
				}
			}
		}

		// Check if we're done with this step (finish_reason indicates completion)
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" && !toolCallsFlushed {
			// Send all accumulated tool calls
			for _, tc := range toolCalls {
				if tc != nil && tc.Name != "" && len(tc.Args) > 0 {
					h.OnToolCall(*tc)
				}
			}
			toolCallsFlushed = true
			// Do not break: we may still receive a final usage chunk
		}
	}

	err := stream.Err()
	dur := time.Since(start)
	base := log.With().
		Str("model", string(params.Model)).
		Int("tools", len(tools)).
		Dur("duration", dur).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Logger()
	if err != nil {
		base.Error().Err(err).Msg("chat_stream_error")
	} else {
		base.Debug().Msg("chat_stream_ok")
	}
	return err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
