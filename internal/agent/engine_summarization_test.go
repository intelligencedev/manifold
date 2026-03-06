package agent

import (
	"context"
	"testing"

	"manifold/internal/llm"
)

type summaryOnlyProvider struct{}

func (p *summaryOnlyProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: "summary"}, nil
}

func (p *summaryOnlyProvider) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return nil
}

func TestMaybeSummarizeKeepsLatestUserInRecentTail(t *testing.T) {
	t.Parallel()

	eng := &Engine{
		LLM:                             &summaryOnlyProvider{},
		SummaryEnabled:                  true,
		ContextWindowTokens:             120,
		SummaryReserveBufferTokens:      40,
		SummaryMinKeepLastMessages:      2,
		SummaryMaxSummaryChunkTokens:    256,
		TokenizationFallbackToHeuristic: true,
	}

	msgs := []llm.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: historyContextPrefix + "Older request that should be summarized."},
		{Role: "assistant", Content: "Older answer that should be summarized."},
		{Role: "user", Content: currentRequestPrefix + "Research plan mode behavior."},
		{Role: "assistant", Content: "Let me search for that.", ToolCalls: []llm.ToolCall{{Name: "github_search_code", ID: "call_1"}}},
		{Role: "tool", ToolID: "call_1", Content: "{\"total_count\": 362, \"items\": [\"...\"]}"},
	}

	summarized := eng.maybeSummarize(context.Background(), msgs)

	if len(summarized) == 0 {
		t.Fatalf("expected summarized messages")
	}

	foundUser := false
	for _, m := range summarized {
		if m.Role == "user" {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Fatalf("expected at least one user message after summarization, got roles: %#v", summarized)
	}

	latestUserIdx := -1
	for i := len(summarized) - 1; i >= 0; i-- {
		if summarized[i].Role == "user" {
			latestUserIdx = i
			break
		}
	}
	if latestUserIdx == -1 {
		t.Fatalf("expected latest user message to be present")
	}
	if latestUserIdx+2 >= len(summarized) {
		t.Fatalf("expected assistant+tool to remain after latest user, got %#v", summarized)
	}
	if summarized[latestUserIdx+1].Role != "assistant" || summarized[latestUserIdx+2].Role != "tool" {
		t.Fatalf("expected assistant/tool to follow latest user, got %#v", summarized)
	}
}
