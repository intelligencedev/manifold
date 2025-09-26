package memory

import (
	"context"
	"testing"
	"time"

	"intelligence.dev/internal/config"
	"intelligence.dev/internal/llm"
	"intelligence.dev/internal/persistence"
	"intelligence.dev/internal/persistence/databases"
)

type stubLLM struct {
	response string
}

func (s *stubLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: s.response}, nil
}

func (s *stubLLM) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	return nil
}

func TestManagerBuildContextWithSummary(t *testing.T) {
	ctx := context.Background()
	mgr, err := databases.NewManager(ctx, config.DBConfig{Chat: config.ChatConfig{Backend: "memory"}})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	store := mgr.Chat

	if _, err := store.EnsureSession(ctx, "sess", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	now := time.Now().UTC()
	turns := []struct {
		user      string
		assistant string
	}{
		{"u1", "a1"},
		{"u2", "a2"},
		{"u3", "a3"},
	}
	for i, turn := range turns {
		messages := []persistence.ChatMessage{
			{Role: "user", Content: turn.user, CreatedAt: now.Add(time.Duration(i*2) * time.Second)},
			{Role: "assistant", Content: turn.assistant, CreatedAt: now.Add(time.Duration(i*2+1) * time.Second)},
		}
		if err := store.AppendMessages(ctx, "sess", messages, turn.assistant, "model"); err != nil {
			t.Fatalf("AppendMessages: %v", err)
		}
	}

	manager := NewManager(store, &stubLLM{response: "summary"}, Config{Enabled: true, Threshold: 4, KeepLast: 2, SummaryModel: "stub"})
	history, err := manager.BuildContext(ctx, "sess")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 messages (summary + 2 turns), got %d", len(history))
	}
	if history[0].Role != "system" || history[0].Content == "" {
		t.Fatalf("expected system summary message, got %#v", history[0])
	}
	if history[1].Content != "u3" || history[2].Content != "a3" {
		t.Fatalf("unexpected tail messages: %#v", history[1:])
	}

	session, ok, err := store.GetSession(ctx, "sess")
	if err != nil || !ok {
		t.Fatalf("GetSession: %v ok=%v", err, ok)
	}
	if session.Summary == "" {
		t.Fatalf("expected summary to be stored")
	}
	if session.SummarizedCount != 4 {
		t.Fatalf("expected summarized count 4, got %d", session.SummarizedCount)
	}
}
