package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/agent/memory"
	"manifold/internal/config"
	openaillm "manifold/internal/llm/openai"
	"manifold/internal/persistence"
)

type stubDebugEvolvingStore struct {
	sessionIDs []string
}

func TestHandleDebugMemoryExplainReturnsScoreComponents(t *testing.T) {
	t.Parallel()

	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn: func(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
			out := make([][]float32, len(texts))
			for i := range texts {
				out[i] = []float32{1, 0}
			}
			return out, nil
		},
		TopK:      1,
		EnableRAG: true,
	})
	if err := em.EvolveEnhanced(context.Background(), "explain query", "ok", "success", &memory.StructuredFeedback{Type: memory.FeedbackSuccess}, nil, ""); err != nil {
		t.Fatalf("EvolveEnhanced failed: %v", err)
	}

	a := &app{
		cfg: &config.Config{},
		userEvolving: map[int64]map[string]*memory.EvolvingMemory{
			systemUserID: {normalizeClientChatSessionID("default"): em},
		},
		evolvingLastUsed: map[int64]map[string]time.Time{
			systemUserID: {normalizeClientChatSessionID("default"): time.Now()},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/memory/explain?query=explain+query", nil)
	a.handleDebugMemoryExplain(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp debugMemoryExplainResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.Enabled || len(resp.Explanations) != 1 {
		t.Fatalf("expected one explanation, got %#v", resp)
	}
	if resp.Explanations[0].Similarity == 0 || resp.Explanations[0].Composite == 0 {
		t.Fatalf("expected score components, got %#v", resp.Explanations[0])
	}
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

func TestHandleDebugMemorySessionDetailReturnsPlainSummary(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	chatStore.sessions["sess-plain"] = persistence.ChatSession{
		ID:              "sess-plain",
		Name:            "Plain Summary Session",
		Summary:         `{"compaction":"{\"type\":\"compaction\",\"encrypted_content\":\"opaque\"}","plain":"plain summary text"}`,
		SummarizedCount: 2,
	}
	chatStore.messages["sess-plain"] = []persistence.ChatMessage{
		{Role: "user", Content: "first message"},
		{Role: "assistant", Content: "second message"},
	}

	app := newDebugMemoryTestApp(t)
	app.cfg.Auth.Enabled = false
	app.chatStore = chatStore
	app.chatMemory = memory.NewManager(chatStore, nil, memory.Config{Enabled: false, ContextWindowTokens: 1024, ReserveBufferTokens: 64})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/memory/sessions/sess-plain", nil)

	app.handleDebugMemorySessionDetail(rec, req, "sess-plain")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp debugMemorySessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Summary != "plain summary text" {
		t.Fatalf("expected plain summary text, got %q", resp.Summary)
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
	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{Name: "lead", Provider: "anthropic", Model: "claude-3-7-sonnet"})
	if err != nil {
		t.Fatalf("upsert lead: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 7, persistence.Specialist{Name: "member-a", Provider: "openai", Model: "gpt-4.1-mini"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 7, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead", "member-a"},
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
	provider := openaillm.New(config.OpenAIConfig{APIKey: "test", BaseURL: "https://api.openai.com/v1", Model: "gpt-5.4", API: "responses"}, nil)
	app.llm = provider
	app.engine.LLM = provider
	return app
}
