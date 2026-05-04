package budget
package budget

import (
	"strings"
	"testing"

	"manifold/internal/llm"
)

func TestFit_TruncatesOversizedMessages(t *testing.T) {
	bigBlob := strings.Repeat("x", 10_000)
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "tool", Content: bigBlob, ToolID: "t1"},
	}

	out := Fit(msgs, 0, 0, 1000)
	if len([]rune(out[2].Content)) > 1100 {
		t.Fatalf("expected tool content truncated to ~1000 runes, got %d", len([]rune(out[2].Content)))
	}
	if !strings.Contains(out[2].Content, "[TRUNCATED]") {
		t.Fatalf("expected truncation marker, got %q", out[2].Content)
	}
	if out[0].Content != "sys" {
		t.Fatalf("system message must not be touched")
	}
}

func TestFit_DropsOldestUntilBudget(t *testing.T) {
	// Each tool message ~ 4000 tokens (~16k runes).
	blob := strings.Repeat("y", 16_000)
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "tool", Content: blob, ToolID: "t1"},
		{Role: "tool", Content: blob, ToolID: "t2"},
		{Role: "user", Content: "u-final"},
	}

	out := Fit(msgs, 6_000, 1_000, 0)
	// Final user must be preserved.
	found := false
	for _, m := range out {
		if m.Role == "user" && m.Content == "u-final" {
			found = true
		}
	}
	if !found {
		t.Fatalf("final user message must be preserved")
	}
	if EstimateTokens(out) > 5_000 {
		t.Fatalf("expected msgs under 5000 tokens after fit, got %d", EstimateTokens(out))
	}
}

func TestFit_TruncatesProtectedUserAsLastResort(t *testing.T) {
	// Single huge user message that itself exceeds the budget.
	blob := strings.Repeat("z", 200_000) // ~50k tokens
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: blob},
	}
	out := Fit(msgs, 8_000, 1_000, 0)
	if EstimateTokens(out) > 7_500 {
		t.Fatalf("expected fit to truncate user message, got %d tokens", EstimateTokens(out))
	}
	if !strings.Contains(out[len(out)-1].Content, "[TRUNCATED]") {
		t.Fatalf("expected truncation marker on user message")
	}
}

func TestFit_NoOpWhenUnderBudget(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
	}
	out := Fit(msgs, 100_000, 25_000, 0)
	if len(out) != len(msgs) {
		t.Fatalf("expected unchanged length, got %d", len(out))
	}
	for i := range msgs {
		if out[i].Content != msgs[i].Content {
			t.Fatalf("message %d unexpectedly modified", i)
		}
	}
}
