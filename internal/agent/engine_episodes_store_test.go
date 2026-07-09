package agent

import (
	"context"
	"testing"

	"manifold/internal/agent/memory"
)

func TestStoreExperienceSkipsTrivialUserInput(t *testing.T) {
	t.Parallel()

	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		EnableRAG: false,
		LLM:       nil,
	})
	e := &Engine{EvolvingMemory: em}
	if id := e.storeExperience(context.Background(), "thanks", "you are welcome", nil, nil); id != "" {
		t.Fatalf("expected empty id for trivial input, got %q", id)
	}
	if got := len(em.ExportMemories()); got != 0 {
		t.Fatalf("expected no stored memory, got %d", got)
	}
}
