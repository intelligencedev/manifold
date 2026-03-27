package agentd

import (
	"context"
	"testing"
	"time"

	"manifold/internal/agent/memory"
	"manifold/internal/llm"
)

type stubLLMProvider struct{}

func (stubLLMProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{}, nil
}

func (stubLLMProvider) ChatStream(context.Context, []llm.Message, []llm.ToolSchema, string, llm.StreamHandler) error {
	return nil
}

func TestCleanupExpiredEvolvingSessionsRemovesExpiredEntries(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	a := &app{
		evolvingSessionTTL: time.Minute,
		userEvolving: map[int64]map[string]*memory.EvolvingMemory{
			7: {"old": memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{})},
		},
		evolvingLastUsed: map[int64]map[string]time.Time{
			7: {"old": now.Add(-2 * time.Minute)},
		},
	}

	removed := a.cleanupExpiredEvolvingSessions(now)
	if removed != 1 {
		t.Fatalf("expected 1 removed session, got %d", removed)
	}
	if sessions := a.userEvolving[7]; len(sessions) != 0 {
		t.Fatalf("expected expired session to be removed, got %#v", sessions)
	}
	if sessions := a.evolvingLastUsed[7]; len(sessions) != 0 {
		t.Fatalf("expected last-used entry to be removed, got %#v", sessions)
	}
}

func TestGetOrCreateEvolvingMemoryForSessionUpdatesLastUsed(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Add(-10 * time.Minute)
	a := &app{
		evolvingCfg: memory.EvolvingMemoryConfig{LLM: stubLLMProvider{}},
		userEvolving: map[int64]map[string]*memory.EvolvingMemory{
			7: {"sess": memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{})},
		},
		evolvingLastUsed: map[int64]map[string]time.Time{
			7: {"sess": now},
		},
	}

	em := a.getOrCreateEvolvingMemoryForSession(7, "sess")
	if em == nil {
		t.Fatal("expected existing evolving memory to be returned")
	}
	updated := a.evolvingLastUsed[7]["sess"]
	if !updated.After(now) {
		t.Fatalf("expected last-used timestamp to be refreshed, got %v <= %v", updated, now)
	}
}
