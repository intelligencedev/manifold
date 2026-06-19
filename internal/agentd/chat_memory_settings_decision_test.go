package agentd

import (
	"testing"

	"manifold/internal/agent"
	"manifold/internal/agent/memory/artifact"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/persistence/databases"
)

func TestApplyChatMemorySettingsDisablesDecisionState(t *testing.T) {
	t.Parallel()

	store := databases.NewMemoryDecisionStore()
	eng := &agent.Engine{
		DecisionStore:     store,
		DecisionDistiller: decision.SimpleDistiller{},
		DecisionService:   &decision.Service{Store: store},
		ArtifactCapture:   &artifact.CaptureManager{},
	}
	applyChatMemorySettingsToEngine(eng, chatMemoryRunSettings{MemoryEnabled: false})

	if eng.DecisionStore != nil {
		t.Fatal("expected DecisionStore cleared when session memory is disabled")
	}
	if eng.DecisionDistiller != nil {
		t.Fatal("expected DecisionDistiller cleared when session memory is disabled")
	}
	if eng.DecisionService != nil {
		t.Fatal("expected DecisionService cleared when session memory is disabled")
	}
	if eng.ArtifactCapture != nil {
		t.Fatal("expected ArtifactCapture cleared when session memory is disabled")
	}
}

func TestApplyChatMemorySettingsKeepsDecisionStateWhenMemoryEnabled(t *testing.T) {
	t.Parallel()

	store := databases.NewMemoryDecisionStore()
	eng := &agent.Engine{
		DecisionStore:     store,
		DecisionDistiller: decision.SimpleDistiller{},
		DecisionService:   &decision.Service{Store: store},
	}
	applyChatMemorySettingsToEngine(eng, chatMemoryRunSettings{
		MemoryEnabled:         true,
		EvolvingMemoryEnabled: true,
		BeliefMemoryEnabled:   true,
	})

	if eng.DecisionStore == nil || eng.DecisionDistiller == nil || eng.DecisionService == nil {
		t.Fatalf("expected decision state preserved when memory is enabled, got %#v", eng)
	}
}
