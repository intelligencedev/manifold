package openai

import (
    "context"
    "encoding/json"

    sdk "github.com/openai/openai-go/v2"
    "github.com/openai/openai-go/v2/option"
    "net/http"

    "gptagent/internal/config"
    "gptagent/internal/llm"
)

type Client struct {
    sdk    sdk.Client
    model  string
}

func New(c config.OpenAIConfig, httpClient *http.Client) *Client {
    opts := []option.RequestOption{option.WithAPIKey(c.APIKey)}
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
    // tools
    params.Tools = AdaptSchemas(tools)
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

// ChatStream is stubbed for now. The TUI can directly use openai-go SDK if needed.
func (c *Client) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
    // A minimal non-streaming fallback: call Chat and emit the full message as one delta.
    m, err := c.Chat(ctx, msgs, tools, model)
    if err != nil {
        return err
    }
    if m.Content != "" {
        h.OnDelta(m.Content)
    }
    for _, tc := range m.ToolCalls {
        h.OnToolCall(tc)
    }
    return nil
}

func firstNonEmpty(vals ...string) string {
    for _, v := range vals {
        if v != "" { return v }
    }
    return ""
}
