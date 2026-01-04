package memory

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"

	"manifold/internal/config"
	"manifold/internal/llm"
)

// mockLLMProvider is a simple mock for testing
type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	return llm.Message{
		Role:    "assistant",
		Content: m.response,
	}, nil
}

func (m *mockLLMProvider) ChatStream(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, model string, handler llm.StreamHandler) error {
	handler.OnDelta(m.response)
	return nil
}

func TestEvolvingMemory_SearchSynthesizeEvolve(t *testing.T) {
	ctx := context.Background()

	// Load .env file (fallback to example.env) for embedding config
	_ = godotenv.Load("../../../.env")
	_ = godotenv.Load("../../../example.env")

	// Create embedding config from environment
	embedCfg := config.EmbeddingConfig{
		BaseURL:   os.Getenv("EMBED_BASE_URL"),
		Path:      os.Getenv("EMBED_PATH"),
		Model:     os.Getenv("EMBED_MODEL"),
		APIKey:    os.Getenv("EMBED_API_KEY"),
		APIHeader: os.Getenv("EMBED_API_HEADER"),
	}

	// Skip if embedding service not configured
	if embedCfg.BaseURL == "" || embedCfg.APIKey == "" {
		t.Skip("Embedding service not configured (EMBED_BASE_URL or EMBED_API_KEY missing)")
	}

	mockLLM := &mockLLMProvider{response: "Key lesson: Always check inputs first."}

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: embedCfg,
		LLM:             mockLLM,
		Model:           "test-model",
		MaxSize:         100,
		TopK:            3,
		WindowSize:      10,
		EnableRAG:       true,
	})

	// Test Evolve (adding memories)
	t.Run("Evolve", func(t *testing.T) {
		err := em.Evolve(ctx, "test task 1", "solution 1", "success")
		if err != nil {
			t.Fatalf("Evolve failed: %v", err)
		}

		if len(em.entries) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(em.entries))
		}
	})

	// Test ExpRecent
	t.Run("ExpRecent", func(t *testing.T) {
		// Clear existing entries and add fresh test entries
		em.entries = make([]*MemoryEntry, 0)
		for i := 0; i < 5; i++ {
			em.entries = append(em.entries, &MemoryEntry{
				Input:    "task " + string(rune('A'+i)),
				Output:   "output " + string(rune('A'+i)),
				Feedback: "success",
			})
		}

		context := em.BuildExpRecentContext()
		if context == "" {
			t.Error("ExpRecent context should not be empty")
		}
		if len(em.GetRecentWindow()) != 5 {
			t.Errorf("Expected 5 recent entries, got %d", len(em.GetRecentWindow()))
		}
	})

	// Test memory pruning
	t.Run("PruneOnMaxSize", func(t *testing.T) {
		em.maxSize = 3
		em.entries = make([]*MemoryEntry, 0)

		for i := 0; i < 5; i++ {
			em.entries = append(em.entries, &MemoryEntry{ID: string(rune('A' + i))})
		}

		// Manually trigger pruning
		if len(em.entries) > em.maxSize {
			em.entries = em.entries[len(em.entries)-em.maxSize:]
		}

		if len(em.entries) != 3 {
			t.Errorf("Expected 3 entries after pruning, got %d", len(em.entries))
		}
	})

	// Test ApplyEdits
	t.Run("ApplyEdits", func(t *testing.T) {
		em.entries = []*MemoryEntry{
			{ID: "mem1", Summary: "test1"},
			{ID: "mem2", Summary: "test2"},
			{ID: "mem3", Summary: "test3"},
		}

		ops := []MemoryEditOp{
			{Type: "PRUNE", IDs: []string{"mem2"}},
			{Type: "UPDATE_TAG", IDs: []string{"mem1"}, Tag: "important"},
		}

		err := em.ApplyEdits(ctx, ops)
		if err != nil {
			t.Fatalf("ApplyEdits failed: %v", err)
		}

		if len(em.entries) != 2 {
			t.Errorf("Expected 2 entries after prune, got %d", len(em.entries))
		}

		if em.entries[0].Metadata["tag"] != "important" {
			t.Error("Tag was not updated")
		}
	})
}

func TestReMemController(t *testing.T) {
	ctx := context.Background()

	// Load .env file (fallback to example.env) for embedding config
	_ = godotenv.Load("../../../.env")
	_ = godotenv.Load("../../../example.env")

	// Create embedding config from environment
	embedCfg := config.EmbeddingConfig{
		BaseURL:   os.Getenv("EMBED_BASE_URL"),
		Path:      os.Getenv("EMBED_PATH"),
		Model:     os.Getenv("EMBED_MODEL"),
		APIKey:    os.Getenv("EMBED_API_KEY"),
		APIHeader: os.Getenv("EMBED_API_HEADER"),
	}

	// Skip if embedding service not configured
	if embedCfg.BaseURL == "" || embedCfg.APIKey == "" {
		t.Skip("Embedding service not configured (EMBED_BASE_URL or EMBED_API_KEY missing)")
	}

	mockLLM := &mockLLMProvider{
		response: `{"action":"ACT","content":"Final answer: 42"}`,
	}

	em := NewEvolvingMemory(EvolvingMemoryConfig{
		EmbeddingConfig: embedCfg,
		LLM:             mockLLM,
		Model:           "test-model",
		MaxSize:         100,
		TopK:            3,
		WindowSize:      10,
		EnableRAG:       true,
	})

	rc := NewReMemController(ReMemConfig{
		Memory:        em,
		LLM:           mockLLM,
		Model:         "test-model",
		MaxInnerSteps: 5,
	})

	t.Run("Execute", func(t *testing.T) {
		finalContent, trace, err := rc.Execute(ctx, "What is the answer?", nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if finalContent != "Final answer: 42" {
			t.Errorf("Expected 'Final answer: 42', got '%s'", finalContent)
		}

		if len(trace) != 0 {
			t.Logf("Reasoning trace: %v", trace)
		}
	})
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
