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

	// no system or history - no annotations needed
	msgs = BuildInitialLLMMessages("", "only", nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	// Without history, no annotation should be added
	if strings.Contains(msgs[0].Content, "[CURRENT REQUEST]") {
		t.Fatalf("should not have annotation without history: %s", msgs[0].Content)
	}
	if msgs[0].Content != "only" {
		t.Fatalf("unexpected single message content: %s", msgs[0].Content)
	}
}

func TestBuildInitialLLMMessagesNoAnnotationWithoutHistory(t *testing.T) {
	// When there's no history, the user message should NOT be annotated
	msgs := BuildInitialLLMMessages("system prompt", "hello", nil)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Content != "hello" {
		t.Fatalf("user message should be unannotated without history: %s", msgs[1].Content)
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

func TestFormatHistorySummary(t *testing.T) {
	// Empty history
	summary := FormatHistorySummary(nil)
	if summary != "(no history)" {
		t.Fatalf("expected no history message, got: %s", summary)
	}

	// With history
	hist := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	summary = FormatHistorySummary(hist)
	if !strings.Contains(summary, "2 messages") {
		t.Fatalf("expected message count in summary: %s", summary)
	}
	if !strings.Contains(summary, "user") || !strings.Contains(summary, "assistant") {
		t.Fatalf("expected roles in summary: %s", summary)
	}
}
