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
				// Always set content for assistant messages with tool calls
				content := m.Content
				if content == "" {
					content = " " // Use a space instead of empty string to avoid template errors
				}
				unionMsg := sdk.AssistantMessage(content)
				asst := unionMsg.OfAssistant

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
				out = append(out, unionMsg)
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
