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
    for _, m := range msgs {
        switch m.Role {
        case "system":
            params.Messages = append(params.Messages, sdk.SystemMessage(m.Content))
        case "user":
            params.Messages = append(params.Messages, sdk.UserMessage(m.Content))
        case "assistant":
            if len(m.ToolCalls) == 0 {
                params.Messages = append(params.Messages, sdk.AssistantMessage(m.Content))
            } else {
                var asst sdk.ChatCompletionAssistantMessageParam
                if m.Content != "" {
                    asst.Content.OfString = sdk.String(m.Content)
                }
                for _, tc := range m.ToolCalls {
                    // function tool call variant
                    fn := sdk.ChatCompletionMessageFunctionToolCallParam{
                        ID: tc.ID,
                        Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{
                            Arguments: string(tc.Args),
                            Name:      tc.Name,
                        },
                    }
                    asst.ToolCalls = append(asst.ToolCalls, sdk.ChatCompletionMessageToolCallUnionParam{OfFunction: &fn})
                }
                params.Messages = append(params.Messages, sdk.ChatCompletionMessageParamUnion{OfAssistant: &asst})
            }
        case "tool":
            params.Messages = append(params.Messages, sdk.ToolMessage(m.Content, m.ToolID))
        }
    }
    // tools
    for _, t := range tools {
        def := sdk.FunctionDefinitionParam{
            Name:        t.Name,
            Description: sdk.String(t.Description),
            Parameters:  t.Parameters,
        }
        params.Tools = append(params.Tools, sdk.ChatCompletionFunctionTool(def))
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
        out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
            Name: tc.Function.Name,
            Args: json.RawMessage(tc.Function.Arguments),
            ID:   tc.ID,
        })
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
