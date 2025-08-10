package openai

import (
    sdk "github.com/openai/openai-go/v2"

    "gptagent/internal/llm"
)

// AdaptSchemas converts internal llm.ToolSchema definitions into OpenAI SDK tool params.
func AdaptSchemas(schemas []llm.ToolSchema) []sdk.ChatCompletionToolUnionParam {
    out := make([]sdk.ChatCompletionToolUnionParam, 0, len(schemas))
    for _, s := range schemas {
        def := sdk.FunctionDefinitionParam{
            Name:        s.Name,
            Description: sdk.String(s.Description),
            Parameters:  s.Parameters,
        }
        out = append(out, sdk.ChatCompletionFunctionTool(def))
    }
    return out
}

// AdaptMessages converts portable llm.Message history to OpenAI SDK message params.
func AdaptMessages(msgs []llm.Message) []sdk.ChatCompletionMessageParamUnion {
    out := make([]sdk.ChatCompletionMessageParamUnion, 0, len(msgs))
    for _, m := range msgs {
        switch m.Role {
        case "system":
            out = append(out, sdk.SystemMessage(m.Content))
        case "user":
            out = append(out, sdk.UserMessage(m.Content))
        case "assistant":
            if len(m.ToolCalls) == 0 {
                out = append(out, sdk.AssistantMessage(m.Content))
            } else {
                var asst sdk.ChatCompletionAssistantMessageParam
                if m.Content != "" {
                    asst.Content.OfString = sdk.String(m.Content)
                }
                for _, tc := range m.ToolCalls {
                    fn := sdk.ChatCompletionMessageFunctionToolCallParam{
                        ID: tc.ID,
                        Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{
                            Arguments: string(tc.Args),
                            Name:      tc.Name,
                        },
                    }
                    asst.ToolCalls = append(asst.ToolCalls, sdk.ChatCompletionMessageToolCallUnionParam{OfFunction: &fn})
                }
                out = append(out, sdk.ChatCompletionMessageParamUnion{OfAssistant: &asst})
            }
        case "tool":
            out = append(out, sdk.ToolMessage(m.Content, m.ToolID))
        }
    }
    return out
}
