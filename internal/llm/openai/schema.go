package openai

import (
	"encoding/json"
	"strings"

	sdk "github.com/openai/openai-go/v2"

	"manifold/internal/llm"
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

func isGemini3Model(model string) bool { return strings.HasPrefix(strings.ToLower(model), "gemini-3") }

// AdaptMessages converts portable llm.Message history to OpenAI SDK message params.
func AdaptMessages(model string, msgs []llm.Message) []sdk.ChatCompletionMessageParamUnion {
	out := make([]sdk.ChatCompletionMessageParamUnion, 0, len(msgs))
	gemini := isGemini3Model(model)
	for _, m := range msgs {
		switch m.Role {
		case "system":
			// Ensure system messages always have content
			content := m.Content
			if content == "" {
				content = "You are a helpful assistant." // Default system message
			}
			out = append(out, sdk.SystemMessage(content))
		case "user":
			// Ensure user messages always have content
			content := m.Content
			if content == "" {
				content = " " // Use a space instead of empty string
			}
			out = append(out, sdk.UserMessage(content))
		case "assistant":
			if len(m.ToolCalls) == 0 {
				// Ensure assistant messages always have content, even if empty
				content := m.Content
				if content == "" {
					content = " " // Use a space instead of empty string to avoid template errors
				}
				out = append(out, sdk.AssistantMessage(content))
			} else {
				var asst sdk.ChatCompletionAssistantMessageParam
				// Always set content for assistant messages with tool calls
				content := m.Content
				if content == "" {
					content = " " // Use a space instead of empty string to avoid template errors
				}
				asst.Content.OfString = sdk.String(content)

				for _, tc := range m.ToolCalls {
					if gemini && strings.TrimSpace(tc.ThoughtSignature) != "" {
						raw := map[string]any{
							"id":   tc.ID,
							"type": "function",
							"function": map[string]any{
								"arguments": string(tc.Args),
								"name":      tc.Name,
							},
							"extra_content": map[string]any{
								"google": map[string]any{"thought_signature": tc.ThoughtSignature},
							},
						}
						// Fallback: SDK may not expose param.Override (version mismatch). Embed extra content as part of arguments JSON.
						b, _ := json.Marshal(raw)
						fn := sdk.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{
								Arguments: string(b),
								Name:      tc.Name,
							},
						}
						asst.ToolCalls = append(asst.ToolCalls, sdk.ChatCompletionMessageToolCallUnionParam{OfFunction: &fn})
						continue
					}
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
			// Ensure tool messages always have valid content
			content := m.Content
			if content == "" {
				content = `{"error": "empty tool response"}` // Provide a default JSON response
			}
			out = append(out, sdk.ToolMessage(content, m.ToolID))
		}
	}
	return out
}
