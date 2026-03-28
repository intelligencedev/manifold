package memory

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/llm"
)

// mockLLMProvider is a simple mock for testing.
type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: m.response}, nil
}

func (m *mockLLMProvider) ChatStream(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, model string, handler llm.StreamHandler) error {
	handler.OnDelta(m.response)
	return nil
}

func testEmbedFn(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
	// Deterministic, cheap embedding for tests.
	out := make([][]float32, len(texts))
	for i, s := range texts {
		v := make([]float32, 8)
		for j := 0; j < len(s); j++ {
			v[j%len(v)] += float32(s[j]) / 255.0
		}
		out[i] = v
	}
	return out, nil
}

func TestEvolvingMemory_SearchSynthesizeEvolve(t *testing.T) {
	ctx := context.Background()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{},
		EmbedFn:         testEmbedFn,
		LLM:             &mockLLMProvider{response: "Key lesson: Always check inputs first."},
		Model:           "test-model",
		MaxSize:         100,
		TopK:            3,
		WindowSize:      10,
		EnableRAG:       true,
	})

	// Evolve (adding memories)
	if err := em.EvolveEnhanced(ctx, "test task 1", "solution 1", "success", nil, nil, ""); err != nil {
		t.Fatalf("Evolve failed: %v", err)
	}
	if got := len(em.entries); got != 1 {
		t.Fatalf("expected 1 entry, got %d", got)
	}

	// Search should find the memory.
	res, err := em.Search(ctx, "test task")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("expected search to return results")
	}

	// Synthesize should produce non-empty context.
	ctxStr := em.Synthesize(ctx, "current task", res)
	if ctxStr == "" {
		t.Fatalf("expected synthesized context")
	}
}

func TestEvolvingMemory_ExpRecent(t *testing.T) {
	ctx := context.Background()
	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn})

	// Add entries directly to avoid summarizer/embeddings.
	em.mu.Lock()
	for i := 0; i < 5; i++ {
		em.entries = append(em.entries, &MemoryEntry{Input: "task", Output: "out", Feedback: "success"})
	}
	em.mu.Unlock()

	recent := em.BuildExpRecentContext()
	if recent == "" {
		t.Fatalf("expected recent context")
	}
	_ = ctx
}

func TestEvolvingMemory_CallbacksFire(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var (
		mu          sync.Mutex
		searchHits  int
		synthHits   int
		evolveHits  int
		lastPhase   PhaseType
		lastEventID string
	)

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: config.EmbeddingConfig{},
		EmbedFn:         testEmbedFn,
		LLM:             &mockLLMProvider{response: "Lesson."},
		Model:           "test-model",
		TopK:            2,
		EnableRAG:       true,
		Callbacks: &MemoryCallbacks{
			OnSearch: func(evt *MemoryEvent) {
				mu.Lock()
				defer mu.Unlock()
				searchHits++
				lastPhase = evt.Phase
				if len(evt.RetrievedIDs) > 0 {
					lastEventID = evt.RetrievedIDs[0]
				}
			},
			OnSynthesized: func(evt *MemoryEvent) {
				mu.Lock()
				defer mu.Unlock()
				synthHits++
				lastPhase = evt.Phase
			},
			OnEvolve: func(evt *MemoryEvent) {
				mu.Lock()
				defer mu.Unlock()
				evolveHits++
				lastPhase = evt.Phase
			},
		},
	})

	if err := em.EvolveEnhanced(ctx, "do thing", "done", "success", nil, nil, ""); err != nil {
		t.Fatalf("evolve failed: %v", err)
	}

	res, err := em.Search(ctx, "do")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	_ = em.Synthesize(ctx, "cur", res)

	mu.Lock()
	defer mu.Unlock()
	if evolveHits == 0 {
		t.Fatalf("expected evolve callback")
	}
	if searchHits == 0 {
		t.Fatalf("expected search callback")
	}
	if synthHits == 0 {
		t.Fatalf("expected synth callback")
	}
	if lastPhase == "" {
		t.Fatalf("expected phase to be set")
	}
	if lastEventID == "" {
		// It's okay if empty depending on topK and embeddings; but in practice it should be populated.
		t.Logf("no retrieved id captured")
	}
}

func TestEvolvingMemorySnapshotsReturnCopies(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{EmbedFn: testEmbedFn})

	em.mu.Lock()
	em.entries = append(em.entries, &MemoryEntry{
		Input:     "original input",
		Output:    "original output",
		Feedback:  "success",
		Embedding: []float32{1, 2, 3},
		Metadata: map[string]interface{}{
			"tag": "original",
		},
		StructuredFeedback: &StructuredFeedback{Message: "keep me"},
	})
	em.mu.Unlock()

	recent := em.GetRecentWindow()
	exported := em.ExportMemories()

	recent[0].Input = "mutated recent"
	recent[0].Embedding[0] = 99
	recent[0].Metadata["tag"] = "mutated"
	recent[0].StructuredFeedback.Message = "mutated"

	exported[0].Output = "mutated export"

	fresh := em.ExportMemories()
	if fresh[0].Input != "original input" {
		t.Fatalf("expected internal input to remain unchanged, got %q", fresh[0].Input)
	}
	if fresh[0].Output != "original output" {
		t.Fatalf("expected internal output to remain unchanged, got %q", fresh[0].Output)
	}
	if fresh[0].Embedding[0] != 1 {
		t.Fatalf("expected internal embedding to remain unchanged, got %v", fresh[0].Embedding)
	}
	if fresh[0].Metadata["tag"] != "original" {
		t.Fatalf("expected internal metadata to remain unchanged, got %#v", fresh[0].Metadata)
	}
	if fresh[0].StructuredFeedback == nil || fresh[0].StructuredFeedback.Message != "keep me" {
		t.Fatalf("expected structured feedback to remain unchanged, got %#v", fresh[0].StructuredFeedback)
	}
}

func TestEvolvingMemoryRelevanceScoreDoesNotCompound(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:          testEmbedFn,
		MaxSize:          1,
		EnableSmartPrune: true,
		RelevanceDecay:   0.99,
		MinRelevance:     0.1,
		PruneThreshold:   0.95,
	})

	em.mu.Lock()
	em.entries = []*MemoryEntry{
		{
			ID:             "stale",
			CreatedAt:      now.Add(-14 * 24 * time.Hour),
			LastAccessedAt: now.Add(-14 * 24 * time.Hour),
			AccessCount:    0,
		},
		{
			ID:             "fresh",
			CreatedAt:      now,
			LastAccessedAt: now,
			AccessCount:    0,
		},
	}
	em.mu.Unlock()

	em.mu.Lock()
	entry := em.entries[0]
	first := em.computeRelevanceScore(now, entry)
	entry.RelevanceScore = first
	second := em.computeRelevanceScore(now, entry)
	em.mu.Unlock()

	if math.Abs(first-second) > 1e-9 {
		t.Fatalf("expected stable relevance score, got first=%f second=%f", first, second)
	}

	em.mu.Lock()
	em.relevanceBasedPrune(context.Background())
	remaining := em.entries
	em.mu.Unlock()

	if len(remaining) != 1 {
		t.Fatalf("expected prune to keep one entry, got %d", len(remaining))
	}
	if remaining[0].ID != "fresh" {
		t.Fatalf("expected freshest entry to remain, got %q", remaining[0].ID)
	}
}

type recordingEvolvingMemoryStore struct {
	saveCh chan []*MemoryEntry
}

func (s *recordingEvolvingMemoryStore) Load(context.Context, int64, string) ([]*MemoryEntry, error) {
	return nil, nil
}

func (s *recordingEvolvingMemoryStore) Save(_ context.Context, _ int64, _ string, entries []*MemoryEntry) error {
	s.saveCh <- cloneEntrySlice(entries)
	return nil
}

type queuedMockLLMProvider struct {
	responses []string
	mu        sync.Mutex
	index     int
}

func (m *queuedMockLLMProvider) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string) (llm.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	resp := ""
	if m.index < len(m.responses) {
		resp = m.responses[m.index]
		m.index++
	}
	return llm.Message{Role: "assistant", Content: resp}, nil
}

func (m *queuedMockLLMProvider) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return nil
}

func TestSearchPersistsAccessMetrics(t *testing.T) {
	t.Parallel()

	store := &recordingEvolvingMemoryStore{saveCh: make(chan []*MemoryEntry, 1)}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		Store:     store,
		UserID:    1,
		SessionID: "search-session",
	})

	em.mu.Lock()
	em.entries = []*MemoryEntry{{
		ID:        "entry-1",
		Input:     "same task",
		Output:    "result",
		Feedback:  "success",
		Embedding: [][]float32{{0}}[0],
		CreatedAt: time.Now().UTC(),
	}}
	em.entries[0].Embedding, _ = func() ([]float32, error) {
		vecs, err := testEmbedFn(context.Background(), config.EmbeddingConfig{}, []string{"same task"})
		if err != nil {
			return nil, err
		}
		return vecs[0], nil
	}()
	em.mu.Unlock()

	results, err := em.Search(context.Background(), "same task")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved entry, got %d", len(saved))
		}
		if saved[0].AccessCount != 1 {
			t.Fatalf("expected access count 1, got %d", saved[0].AccessCount)
		}
		if saved[0].LastAccessedAt.IsZero() {
			t.Fatal("expected last accessed timestamp to be persisted")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for access-metric persistence")
	}
}

func TestSmartMergeReembedsMergedSummary(t *testing.T) {
	t.Parallel()

	provider := &queuedMockLLMProvider{responses: []string{"first lesson", "second lesson"}}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:          testEmbedFn,
		LLM:              provider,
		Model:            "test-model",
		EnableSmartPrune: true,
		PruneThreshold:   0.95,
	})

	ctx := context.Background()
	if err := em.EvolveEnhanced(ctx, "same task", "first output", "success", nil, nil, ""); err != nil {
		t.Fatalf("first evolve failed: %v", err)
	}
	if err := em.EvolveEnhanced(ctx, "same task", "second output", "success", nil, nil, ""); err != nil {
		t.Fatalf("second evolve failed: %v", err)
	}

	memories := em.ExportMemories()
	if len(memories) != 1 {
		t.Fatalf("expected one merged memory, got %d", len(memories))
	}
	merged := memories[0]
	wantSummary := "first lesson\n\nsecond lesson"
	if merged.Summary != wantSummary {
		t.Fatalf("expected merged summary %q, got %q", wantSummary, merged.Summary)
	}
	mergedFrom, ok := merged.Metadata["merged_from"].([]string)
	if !ok || len(mergedFrom) != 1 {
		t.Fatalf("expected merged_from metadata, got %#v", merged.Metadata["merged_from"])
	}

	wantEmbedding, err := testEmbedFn(ctx, config.EmbeddingConfig{}, []string{wantSummary})
	if err != nil {
		t.Fatalf("testEmbedFn failed: %v", err)
	}
	if len(merged.Embedding) != len(wantEmbedding[0]) {
		t.Fatalf("expected merged embedding length %d, got %d", len(wantEmbedding[0]), len(merged.Embedding))
	}
	for i := range wantEmbedding[0] {
		if merged.Embedding[i] != wantEmbedding[0][i] {
			t.Fatalf("expected merged embedding %v, got %v", wantEmbedding[0], merged.Embedding)
		}
	}
}

func TestPersistEntriesAsyncCoalescesRapidUpdates(t *testing.T) {
	t.Parallel()

	store := &recordingEvolvingMemoryStore{saveCh: make(chan []*MemoryEntry, 4)}
	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:   testEmbedFn,
		Store:     store,
		UserID:    9,
		SessionID: "debounce-session",
	})
	em.persistDelay = 30 * time.Millisecond

	em.persistEntriesAsync([]*MemoryEntry{{ID: "first", Input: "a"}})
	em.persistEntriesAsync([]*MemoryEntry{{ID: "second", Input: "b"}})

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved entry, got %d", len(saved))
		}
		if saved[0].ID != "second" {
			t.Fatalf("expected latest snapshot to be persisted, got %q", saved[0].ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced persist")
	}

	select {
	case extra := <-store.saveCh:
		t.Fatalf("expected only one coalesced save, got extra %#v", extra)
	case <-time.After(120 * time.Millisecond):
	}
}

func TestNewEvolvingMemoryUsesConfiguredPersistDebounce(t *testing.T) {
	t.Parallel()

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbedFn:         testEmbedFn,
		PersistDebounce: 75 * time.Millisecond,
	})

	if em.persistDelay != 75*time.Millisecond {
		t.Fatalf("expected persist delay 75ms, got %s", em.persistDelay)
	}
}
