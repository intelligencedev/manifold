package openai

import (
	"context"
	"encoding/json"

	"net/http"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"

	"gptagent/internal/config"
	"gptagent/internal/llm"
)

type Client struct {
	sdk   sdk.Client
	model string
}

func New(c config.OpenAIConfig, httpClient *http.Client) *Client {
	opts := []option.RequestOption{option.WithAPIKey(c.APIKey)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}
	return &Client{sdk: sdk.NewClient(opts...), model: c.Model}
}

// Chat implements llm.Provider.Chat using OpenAI Chat Completions.
func (c *Client) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	// messages
	params.Messages = AdaptMessages(msgs)
	// tools: include only when provided to avoid sending an empty array
	if len(tools) > 0 {
		params.Tools = AdaptSchemas(tools)
	}
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.Message{}, err
	}
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
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	params.Messages = AdaptMessages(msgs)
	if len(tools) > 0 {
		params.Tools = AdaptSchemas(tools)
	}
	if len(extra) > 0 {
		params.SetExtraFields(extra)
	}
	comp, err := c.sdk.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.Message{}, err
	}
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
	params := sdk.ChatCompletionNewParams{
		Model: sdk.ChatModel(firstNonEmpty(model, c.model)),
	}
	// messages
	params.Messages = AdaptMessages(msgs)
	// tools
	params.Tools = AdaptSchemas(tools)

	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	// Accumulate tool calls across chunks since they come incrementally
	toolCalls := make(map[int]*llm.ToolCall)

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
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
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			// Send all accumulated tool calls
			for _, tc := range toolCalls {
				if tc != nil && tc.Name != "" && len(tc.Args) > 0 {
					h.OnToolCall(*tc)
				}
			}
			break // Exit the stream loop when we get a finish reason
		}
	}

	return stream.Err()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
