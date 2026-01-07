package openai

import (
	"encoding/json"
	"testing"

	"manifold/internal/llm"
)

func TestResponsesTokenizer_BuildInputItems_AssistantToolCallWithoutContent(t *testing.T) {
	tokenizer := &ResponsesTokenizer{}
	items, _ := tokenizer.buildInputItems([]llm.Message{
		{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "run", Args: json.RawMessage(`{"cmd":"ls"}`)}},
		},
		{Role: "tool", Content: `{"ok":true}`, ToolID: "call_1"},
	})

	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if obj["type"] == "message" && obj["role"] == "assistant" {
			t.Fatalf("unexpected assistant message without content in input_tokens payload")
		}
	}
}
