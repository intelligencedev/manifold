package agents

import (
	"context"
	"testing"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/tools"
)

type delegatorMemoryProvider struct {
	chatResponse   string
	streamResponse string
}

func (p *delegatorMemoryProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: p.chatResponse}, nil
}

func (p *delegatorMemoryProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, handler llm.StreamHandler) error {
	handler.OnDelta(p.streamResponse)
	return nil
}

type delegatorRecordingStore struct {
	saveCh chan []*memory.MemoryEntry
}

func (s *delegatorRecordingStore) Load(context.Context, int64, string) ([]*memory.MemoryEntry, error) {
	return nil, nil
}

func (s *delegatorRecordingStore) Save(_ context.Context, _ int64, _ string, entries []*memory.MemoryEntry) error {
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

func delegatorTestEmbedFn(_ context.Context, _ config.EmbeddingConfig, texts []string) ([][]float32, error) {
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

func TestDelegatorRunUsesSharedEvolvingMemory(t *testing.T) {
	t.Parallel()

	provider := &delegatorMemoryProvider{chatResponse: "summary", streamResponse: "delegated final"}
	store := &delegatorRecordingStore{saveCh: make(chan []*memory.MemoryEntry, 1)}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EmbedFn:   delegatorTestEmbedFn,
		LLM:       provider,
		Model:     "test-model",
		Store:     store,
		UserID:    7,
		SessionID: "sess-1",
	})

	d := NewDelegator(tools.NewRegistry(), nil, nil, 1)
	d.SetEvolvingMemory(em)
	ctx := tools.WithProvider(context.Background(), provider)

	out, err := d.Run(ctx, agent.DelegateRequest{
		Prompt:    "remember this",
		UserID:    7,
		SessionID: "sess-1",
	}, nil)
	if err != nil {
		t.Fatalf("delegator run failed: %v", err)
	}
	if out != "delegated final" {
		t.Fatalf("expected delegated final output, got %q", out)
	}

	select {
	case saved := <-store.saveCh:
		if len(saved) != 1 {
			t.Fatalf("expected one saved memory, got %d", len(saved))
		}
		if saved[0] == nil {
			t.Fatal("expected saved memory entry")
		}
		if saved[0].Input != "remember this" {
			t.Fatalf("expected saved input remember this, got %q", saved[0].Input)
		}
		if saved[0].Output != "delegated final" {
			t.Fatalf("expected saved output delegated final, got %q", saved[0].Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delegated evolving memory save")
	}
}
