package agent

import (
	"context"
	"testing"
	"time"

	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools"
)

type memoryTestProvider struct {
	chatResponse   string
	streamResponse string
}

func (p *memoryTestProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: p.chatResponse}, nil
}

func (p *memoryTestProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, handler llm.StreamHandler) error {
	handler.OnDelta(p.streamResponse)
	return nil
}

type recordingMemoryStore struct {
	saveCh chan []*memory.MemoryEntry
}

func (s *recordingMemoryStore) Load(context.Context, int64, string) ([]*memory.MemoryEntry, error) {
	return nil, nil
}

func (s *recordingMemoryStore) Save(_ context.Context, _ int64, _ string, entries []*memory.MemoryEntry) error {
	snapshot := make([]*memory.MemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			snapshot = append(snapshot, nil)
			continue
		}
		copyEntry := *entry
		snapshot = append(snapshot, &copyEntry)
	}
	s.saveCh <- snapshot
	return nil
}

func engineTestEmbedFn(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, s := range texts {
		vec := make([]float32, 4)
		for j := 0; j < len(s); j++ {
			vec[j%len(vec)] += float32(s[j]) / 255.0
		}
		out[i] = vec
	}
	return out, nil
}

func TestRunStreamStoresEvolvingMemory(t *testing.T) {
	t.Parallel()

	provider := &memoryTestProvider{
		chatResponse:   "summary",
		streamResponse: "streamed final",
	}
	store := &recordingMemoryStore{saveCh: make(chan []*memory.MemoryEntry, 1)}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn:   engineTestEmbedFn,
		LLM:       provider,
		Model:     "test-model",
		Store:     store,
		UserID:    7,
		SessionID: "session-1",
	})

	eng := &Engine{
		LLM:            provider,
		MaxSteps:       1,
		EvolvingMemory: em,
		Tools:          tools.NewRegistry(),
	}

	final, err := eng.RunStream(context.Background(), "remember this", nil)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}
	if final != "streamed final" {
		t.Fatalf("expected streamed final response, got %q", final)
	}

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved memory, got %d", len(saved))
		}
		if saved[0] == nil {
			t.Fatalf("expected saved memory entry")
		}
		if saved[0].Input != "remember this" {
			t.Fatalf("expected saved input to match prompt, got %q", saved[0].Input)
		}
		if saved[0].Output != "streamed final" {
			t.Fatalf("expected saved output to match final response, got %q", saved[0].Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for evolving memory save after RunStream")
	}
}
