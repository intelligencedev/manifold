package openai

import (
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

// isGemini3Model detects Gemini 3 model identifiers in both bare and provider-prefixed forms.
// Examples that should match:
//
//	gemini-3-pro-preview
//	google/gemini-3-pro-preview
//	GOOGLE/GEMINI-3.5-FLASH (future variants)
//
// It intentionally looks for the substring "gemini-3" to be resilient to prefixes and suffixes.
// We avoid a broader "gemini" match to keep 2.5 series excluded from strict thought signature logic.
func isGemini3Model(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "gemini-3")
}

// AdaptMessages converts portable llm.Message history to OpenAI SDK message params.
func AdaptMessages(model string, msgs []llm.Message) []sdk.ChatCompletionMessageParamUnion {
	out := make([]sdk.ChatCompletionMessageParamUnion, 0, len(msgs))
	// Note: Gemini 3 thought_signature injection is NOT handled here
	// because the SDK doesn't preserve extra_content fields. For Gemini models,
	// use AdaptMessagesRaw to build the raw JSON payload.
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

// AdaptMessagesRaw builds raw message JSON for Gemini models, including thought_signature fields.
// This bypasses the SDK to preserve extra_content which the typed SDK structs don't support.
func AdaptMessagesRaw(model string, msgs []llm.Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	gemini := isGemini3Model(model)
	for _, m := range msgs {
		switch m.Role {
		case "system":
			content := m.Content
			if content == "" {
				content = "You are a helpful assistant."
			}
			out = append(out, map[string]any{
				"role":    "system",
				"content": content,
			})
		case "user":
			content := m.Content
			if content == "" {
				content = " "
			}
			out = append(out, map[string]any{
				"role":    "user",
				"content": content,
			})
		case "assistant":
			if len(m.ToolCalls) == 0 {
				content := m.Content
				if content == "" {
					content = " "
				}
				out = append(out, map[string]any{
					"role":    "assistant",
					"content": content,
				})
			} else {
				content := m.Content
				if content == "" {
					content = " "
				}
				toolCalls := make([]map[string]any, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					call := map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": string(tc.Args),
						},
					}
					if gemini && strings.TrimSpace(tc.ThoughtSignature) != "" {
						call["extra_content"] = map[string]any{
							"google": map[string]any{
								"thought_signature": tc.ThoughtSignature,
							},
						}
					}
					toolCalls = append(toolCalls, call)
				}
				out = append(out, map[string]any{
					"role":       "assistant",
					"content":    content,
					"tool_calls": toolCalls,
				})
			}
		case "tool":
			content := m.Content
			if content == "" {
				content = `{"error": "empty tool response"}`
			}
			out = append(out, map[string]any{
				"role":         "tool",
				"content":      content,
				"tool_call_id": m.ToolID,
			})
		}
	}
	return out
}
