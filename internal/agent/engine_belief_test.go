package agent

import (
	"context"
	"testing"
	"time"

	"manifold/internal/agent/belief"
	"manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/persistence/databases"
	"manifold/internal/tools"
)

type recordingBeliefStore struct {
	belief.NoopStore
	scope   belief.Scope
	episode belief.Episode
}

func TestRunStreamBeliefEpisodeLinksEvolvingEntry(t *testing.T) {
	t.Parallel()

	provider := &memoryTestProvider{chatResponse: "summary", streamResponse: "done"}
	memoryStore := &recordingMemoryStore{saveCh: make(chan []*memory.MemoryEntry, 1)}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn:   engineTestEmbedFn,
		LLM:       provider,
		Model:     "test-model",
		Store:     memoryStore,
		UserID:    7,
		SessionID: "session-1",
	})
	beliefStore := &recordingBeliefStore{}
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       1,
		UserID:         7,
		ProjectID:      "project-1",
		ObjectiveID:    "objective-1",
		SessionID:      "session-1",
		BeliefStore:    beliefStore,
		EvolvingMemory: em,
	}

	if _, err := eng.RunStream(context.Background(), "remember this", nil); err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}
	if beliefStore.episode.EvolvingEntryID == "" {
		t.Fatal("expected belief episode to reserve evolving entry id")
	}

	select {
	case saved := <-memoryStore.saveCh:
		if len(saved) != 1 || saved[0] == nil {
			t.Fatalf("expected one saved evolving memory entry, got %#v", saved)
		}
		if saved[0].ID != beliefStore.episode.EvolvingEntryID {
			t.Fatalf("expected linked evolving entry %q, got saved entry %q", beliefStore.episode.EvolvingEntryID, saved[0].ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for evolving memory save")
	}
}

func (s *recordingBeliefStore) EnsureScope(_ context.Context, scope belief.Scope) (belief.Scope, error) {
	scope.ID = "scope-1"
	s.scope = scope
	return scope, nil
}

func (s *recordingBeliefStore) UpsertEpisode(_ context.Context, episode belief.Episode) (belief.Episode, error) {
	episode.ID = "episode-1"
	s.episode = episode
	return episode, nil
}

func TestRunStreamStoresBeliefEpisode(t *testing.T) {
	t.Parallel()

	provider := &memoryTestProvider{streamResponse: "done"}
	store := &recordingBeliefStore{}
	eng := &Engine{
		LLM:         provider,
		Tools:       tools.NewRegistry(),
		MaxSteps:    1,
		UserID:      7,
		ProjectID:   "project-1",
		ObjectiveID: "objective-1",
		SessionID:   "session-1",
		AgentRole:   "orchestrator",
		BeliefStore: store,
	}

	final, err := eng.RunStream(context.Background(), "do it", nil)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}
	if final != "done" {
		t.Fatalf("expected final response, got %q", final)
	}
	if store.scope.Kind != belief.ScopeKindObjective {
		t.Fatalf("expected objective scope, got %q", store.scope.Kind)
	}
	if store.scope.Path != "project-1/objective-1" {
		t.Fatalf("unexpected scope path %q", store.scope.Path)
	}
	if store.episode.ProjectID != "project-1" || store.episode.ObjectiveID != "objective-1" {
		t.Fatalf("unexpected episode scope: project=%q objective=%q", store.episode.ProjectID, store.episode.ObjectiveID)
	}
	if store.episode.SessionID != "session-1" || store.episode.UserID != 7 {
		t.Fatalf("unexpected episode identity: session=%q user=%d", store.episode.SessionID, store.episode.UserID)
	}
	if store.episode.Outcome != "success" || store.episode.OutcomeSignal != "implicit_success" {
		t.Fatalf("unexpected episode outcome: %q %q", store.episode.Outcome, store.episode.OutcomeSignal)
	}
	if store.episode.EndedAt == nil {
		t.Fatal("expected ended_at to be set")
	}
}

func TestRunStreamAppliesBeliefDistillation(t *testing.T) {
	t.Parallel()

	provider := &memoryTestProvider{streamResponse: "Transit is used for durable shared working memory in this project."}
	store := databases.NewMemoryBeliefStore()
	eng := &Engine{
		LLM:             provider,
		Tools:           tools.NewRegistry(),
		MaxSteps:        1,
		UserID:          7,
		ProjectID:       "project-1",
		ObjectiveID:     "objective-1",
		SessionID:       "session-1",
		BeliefStore:     store,
		BeliefDistiller: belief.SimpleDistiller{},
	}

	if _, err := eng.RunStream(context.Background(), "summarize memory", nil); err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}
	scope, ok, err := store.GetScope(context.Background(), 7, belief.ScopeKindObjective, "project-1/objective-1")
	if err != nil || !ok {
		t.Fatalf("expected objective scope, ok=%v err=%v", ok, err)
	}
	results, err := store.SearchBeliefs(context.Background(), belief.SearchQuery{
		TenantID: 7,
		ScopeIDs: []string{scope.ID},
		Query:    "Transit",
		Statuses: []belief.BeliefStatus{belief.BeliefStatusActive},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchBeliefs: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one distilled belief, got %d", len(results))
	}
	if results[0].Belief.Confidence >= 0.5 {
		t.Fatalf("implicit success should remain low confidence, got %f", results[0].Belief.Confidence)
	}
	if results[0].Belief.EvidenceFor != 1 {
		t.Fatalf("expected one evidence_for count, got %d", results[0].Belief.EvidenceFor)
	}
}

func TestRunStoresErrorBeliefEpisode(t *testing.T) {
	t.Parallel()

	store := &recordingBeliefStore{}
	eng := &Engine{
		LLM:         errorLLM{},
		Tools:       tools.NewRegistry(),
		MaxSteps:    1,
		UserID:      7,
		ProjectID:   "project-1",
		ObjectiveID: "objective-1",
		SessionID:   "session-1",
		BeliefStore: store,
	}

	_, err := eng.Run(context.Background(), "fail", nil)
	if err == nil {
		t.Fatal("expected Run to fail")
	}
	if store.episode.Outcome != "error" || store.episode.OutcomeSignal != "runtime_error" {
		t.Fatalf("unexpected error episode outcome: %q %q", store.episode.Outcome, store.episode.OutcomeSignal)
	}
	if store.episode.Metadata["error"] == "" {
		t.Fatalf("expected error metadata, got %#v", store.episode.Metadata)
	}
}

type errorLLM struct{}

func (errorLLM) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{}, context.Canceled
}

func (errorLLM) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return context.Canceled
}
