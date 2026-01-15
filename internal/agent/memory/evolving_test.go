package memory

import (
	"context"
	"sync"
	"testing"

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
	if err := em.Evolve(ctx, "test task 1", "solution 1", "success"); err != nil {
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

	if err := em.Evolve(ctx, "do thing", "done", "success"); err != nil {
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
