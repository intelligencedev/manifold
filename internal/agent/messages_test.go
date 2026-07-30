package agent

import (
	"strings"
	"testing"

	"manifold/internal/llm"
)

func TestBuildInitialLLMMessages(t *testing.T) {
	hist := []llm.Message{{Role: "user", Content: "prev"}}
	msgs := BuildInitialLLMMessages("sys", "now", hist)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "sys" {
		t.Fatalf("unexpected system msg: %#v", msgs[0])
	}
	// History message should be annotated with [CONVERSATION HISTORY]
	if !strings.Contains(msgs[1].Content, "[CONVERSATION HISTORY]") {
		t.Fatalf("expected history annotation, got: %s", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "prev") {
		t.Fatalf("expected original history content, got: %s", msgs[1].Content)
	}
	// Current message should be annotated with [CURRENT REQUEST]
	if !strings.Contains(msgs[2].Content, "[CURRENT REQUEST]") {
		t.Fatalf("expected current request annotation, got: %s", msgs[2].Content)
	}
	if !strings.Contains(msgs[2].Content, "now") {
		t.Fatalf("expected original user content, got: %s", msgs[2].Content)
	}

	// No history: still annotate current request so minifiers preserve typed body.
	msgs = BuildInitialLLMMessages("", "only", nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "[CURRENT REQUEST]") {
		t.Fatalf("expected current request annotation without history: %s", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "only") || !strings.HasSuffix(msgs[0].Content, "only") {
		t.Fatalf("unexpected single message content: %s", msgs[0].Content)
	}
}

func TestBuildInitialLLMMessagesAlwaysAnnotatesCurrentRequest(t *testing.T) {
	// Live user text is always annotated so lexminify can isolate it from runtime context.
	msgs := BuildInitialLLMMessages("system prompt", "hello", nil)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[1].Content, "[CURRENT REQUEST]") {
		t.Fatalf("expected current request annotation: %s", msgs[1].Content)
	}
	if !strings.HasSuffix(msgs[1].Content, "hello") {
		t.Fatalf("user typed body must remain intact: %s", msgs[1].Content)
	}
}

func TestBuildInitialLLMMessagesHistoryAnnotation(t *testing.T) {
	// Multi-turn history should only annotate the first user message
	hist := []llm.Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
		{Role: "assistant", Content: "second answer"},
	}
	msgs := BuildInitialLLMMessages("sys", "third question", hist)

	// Should have: system + 4 history + 1 current = 6
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(msgs))
	}

	// First history user message should have annotation
	if !strings.Contains(msgs[1].Content, "[CONVERSATION HISTORY]") {
		t.Fatalf("first history message should be annotated: %s", msgs[1].Content)
	}

	// Other history messages should NOT have the annotation prefix
	if strings.Contains(msgs[2].Content, "[CONVERSATION HISTORY]") {
		t.Fatalf("assistant message should not be annotated: %s", msgs[2].Content)
	}
	if strings.Contains(msgs[3].Content, "[CONVERSATION HISTORY]") {
		t.Fatalf("later user message should not be annotated: %s", msgs[3].Content)
	}

	// Current request should be annotated
	if !strings.Contains(msgs[5].Content, "[CURRENT REQUEST]") {
		t.Fatalf("current request should be annotated: %s", msgs[5].Content)
	}
}

func TestBuildInitialLLMMessagesMovesHistorySystemContextToCurrentRequest(t *testing.T) {
	hist := []llm.Message{
		{Role: "system", Content: "Conversation summary (for context only):\nolder summary"},
		{Role: "user", Content: "previous question"},
		{Role: "assistant", Content: "previous answer"},
	}
	msgs := BuildInitialLLMMessages("stable system", "current task", hist)

	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages after moving synthetic system context, got %d: %#v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != "stable system" {
		t.Fatalf("expected stable system message unchanged, got %#v", msgs[0])
	}
	for _, msg := range msgs[1:] {
		if msg.Role == "system" {
			t.Fatalf("did not expect synthetic history system message to remain in history: %#v", msgs)
		}
	}
	current := msgs[len(msgs)-1]
	if current.Role != "user" {
		t.Fatalf("expected current user message, got %#v", current)
	}
	if !IsRuntimeContextMessage(current) || !strings.Contains(current.Content, "Conversation summary (for context only):") {
		t.Fatalf("expected summary context on current request, got %q", current.Content)
	}
	if !strings.Contains(current.Content, "[CURRENT REQUEST]") || !strings.Contains(current.Content, "current task") {
		t.Fatalf("expected current request after moved context, got %q", current.Content)
	}
}

func TestBuildInitialLLMMessagesRuntimeContextStaysWithCurrentUserAfterToolHistory(t *testing.T) {
	hist := []llm.Message{
		{Role: "system", Content: "summary"},
		{Role: "assistant", Content: "searching", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "search"}}},
		{Role: "tool", ToolID: "call_1", Content: "result"},
	}

	msgs := BuildInitialLLMMessages("static system", "latest", hist)
	msgs = AddRuntimeContextToCurrentUserMessage(msgs, "memory context")

	if len(msgs) != 4 {
		t.Fatalf("expected static/tool history/current order, got %#v", msgs)
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected static system first, got %#v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[2].Role != "tool" {
		t.Fatalf("expected prior tool history before current user, got %#v", msgs)
	}
	if msgs[3].Role != "user" || !IsRuntimeContextMessage(msgs[3]) || !strings.Contains(msgs[3].Content, "[CURRENT REQUEST]") {
		t.Fatalf("expected runtime current user last, got %#v", msgs[3])
	}
	if !strings.Contains(msgs[3].Content, "summary") || !strings.Contains(msgs[3].Content, "memory context") {
		t.Fatalf("expected merged runtime context, got %q", msgs[3].Content)
	}
}

func TestPrependToCurrentUserMessage(t *testing.T) {
	msgs := []llm.Message{{Role: "system", Content: "stable"}, {Role: "user", Content: "do work"}}
	msgs = PrependToCurrentUserMessage(msgs, "runtime context")
	if len(msgs) != 2 {
		t.Fatalf("expected message count unchanged, got %d", len(msgs))
	}
	if msgs[0].Content != "stable" {
		t.Fatalf("system message changed: %#v", msgs[0])
	}
	if msgs[1].Content != "runtime context\n\ndo work" {
		t.Fatalf("unexpected user prompt: %q", msgs[1].Content)
	}
}
