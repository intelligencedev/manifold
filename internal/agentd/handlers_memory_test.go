package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/agent/memory"
	"manifold/internal/config"
	openaillm "manifold/internal/llm/openai"
	"manifold/internal/persistence"
	"manifold/internal/specialists"
)

type stubDebugEvolvingStore struct {
	sessionIDs []string
}

func (s *stubDebugEvolvingStore) Load(_ context.Context, _ int64, _ string) ([]*memory.MemoryEntry, error) {
	return nil, nil
}

func (s *stubDebugEvolvingStore) Save(_ context.Context, _ int64, _ string, _ []*memory.MemoryEntry) error {
	return nil
}

func (s *stubDebugEvolvingStore) ListSessions(_ context.Context, _ int64) ([]string, error) {
	return append([]string(nil), s.sessionIDs...), nil
}

func TestHandleDebugMemorySessionsIncludesEvolvingOnlySessions(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	chatStore.sessions["chat-only"] = persistence.ChatSession{ID: "chat-only", Name: "Chat Only"}

	a := &app{
		cfg:       &config.Config{},
		chatStore: chatStore,
		evolvingCfg: memory.EvolvingMemoryConfig{
			Store: &stubDebugEvolvingStore{sessionIDs: []string{"memory-only", "chat-only"}},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/memory/sessions", nil)

	a.handleDebugMemorySessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var sessions []persistence.ChatSession
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	seen := make(map[string]persistence.ChatSession, len(sessions))
	for _, session := range sessions {
		seen[session.ID] = session
	}
	if _, ok := seen["chat-only"]; !ok {
		t.Fatalf("expected chat-backed session to be present: %#v", sessions)
	}
	if _, ok := seen["memory-only"]; !ok {
		t.Fatalf("expected evolving-memory-only session to be present: %#v", sessions)
	}
}

func TestDebugMemoryTargetSupportsCompactionDefaultOrchestrator(t *testing.T) {
	t.Parallel()

	app := newDebugMemoryTestApp(t)
	got, status, err := app.debugMemoryTargetSupportsCompaction(context.Background(), 7, "sess-1", chatDispatchTarget{})
	if err != nil {
		t.Fatalf("debugMemoryTargetSupportsCompaction: %v", err)
	}
	if status != 0 {
		t.Fatalf("unexpected status: %d", status)
	}
	if !got {
		t.Fatal("expected default orchestrator to support compaction")
	}
}

func TestDebugMemoryTargetSupportsCompactionSpecialistOverride(t *testing.T) {
	t.Parallel()

	app := newDebugMemoryTestApp(t)
	ctx := context.Background()
	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{Name: "alpha", Provider: "anthropic", Model: "claude-3-7-sonnet"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	app.invalidateSpecialistsCache(ctx, 7)

	got, status, err := app.debugMemoryTargetSupportsCompaction(ctx, 7, "sess-1", chatDispatchTarget{SpecialistName: "alpha"})
	if err != nil {
		t.Fatalf("debugMemoryTargetSupportsCompaction: %v", err)
	}
	if status != 0 {
		t.Fatalf("unexpected status: %d", status)
	}
	if got {
		t.Fatal("expected anthropic specialist to skip compaction")
	}
}

func TestDebugMemoryTargetSupportsCompactionTeamOverride(t *testing.T) {
	t.Parallel()

	app := newDebugMemoryTestApp(t)
	ctx := context.Background()
	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{Name: "member-a", Provider: "openai", Model: "gpt-4.1-mini"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 7, persistence.SpecialistTeam{
		Name: "ops",
		Orchestrator: persistence.Specialist{
			Name:     specialists.OrchestratorName,
			Provider: "anthropic",
			Model:    "claude-3-7-sonnet",
		},
		Members: []string{"member-a"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	got, status, err := app.debugMemoryTargetSupportsCompaction(ctx, 7, "sess-1", chatDispatchTarget{TeamName: "ops"})
	if err != nil {
		t.Fatalf("debugMemoryTargetSupportsCompaction: %v", err)
	}
	if status != 0 {
		t.Fatalf("unexpected status: %d", status)
	}
	if got {
		t.Fatal("expected anthropic team orchestrator to skip compaction")
	}
}

func newDebugMemoryTestApp(t *testing.T) *app {
	t.Helper()
	app := newChatEngineBuilderTestApp(t)
	provider := openaillm.New(config.OpenAIConfig{APIKey: "test", BaseURL: "http://127.0.0.1:1", Model: "gpt-5.4"}, nil)
	app.llm = provider
	app.engine.LLM = provider
	return app
}
