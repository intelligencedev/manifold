package agent

import (
	"context"
	"testing"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/llm"
	"manifold/internal/persistence/databases"
	"manifold/internal/tools"
)

type highConfidenceDistiller struct{}

func (highConfidenceDistiller) Distill(context.Context, belief.DistillationInput) ([]belief.Candidate, error) {
	statement := belief.NormalizeStatement("Transit coordinates shared state across agents.")
	return []belief.Candidate{{
		Statement:     statement,
		StatementHash: belief.StatementHash(statement),
		Confidence:    0.95,
		Polarity:      belief.EvidencePolarityFor,
		EvidenceNote:  "tests passed",
	}}, nil
}

func TestRunStreamPromotesCorroboratedObjectiveBeliefToProject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := databases.NewMemoryBeliefStore()
	objectiveScope, err := store.EnsureScope(ctx, belief.Scope{TenantID: 7, Kind: belief.ScopeKindObjective, Path: "project-1/objective-1", Label: "objective-1"})
	if err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	statement := "Transit coordinates shared state across agents."
	if _, err := store.UpsertBelief(ctx, belief.Belief{
		TenantID:      7,
		ScopeID:       objectiveScope.ID,
		Statement:     statement,
		StatementHash: belief.StatementHash(statement),
		Confidence:    0.9,
		EvidenceFor:   1,
		Status:        belief.BeliefStatusActive,
	}); err != nil {
		t.Fatalf("seed belief: %v", err)
	}
	provider := &memoryTestProvider{streamResponse: "done"}
	eng := &Engine{
		LLM:                      provider,
		Tools:                    tools.NewRegistry(),
		MaxSteps:                 1,
		UserID:                   7,
		ProjectID:                "project-1",
		ObjectiveID:              "objective-1",
		SessionID:                "session-1",
		BeliefStore:              store,
		BeliefDistiller:          highConfidenceDistiller{},
		BeliefPromotionThreshold: 0.8,
	}

	if _, err := eng.RunStream(ctx, "coordinate", nil); err != nil {
		t.Fatalf("RunStream returned error: %v", err)
	}
	projectScope, ok, err := store.GetScope(ctx, 7, belief.ScopeKindProject, "project-1")
	if err != nil || !ok {
		t.Fatalf("expected project scope, ok=%v err=%v", ok, err)
	}
	results, err := store.SearchBeliefs(ctx, belief.SearchQuery{TenantID: 7, ScopeIDs: []string{projectScope.ID}, Query: "Transit", Statuses: []belief.BeliefStatus{belief.BeliefStatusActive}, Limit: 10})
	if err != nil {
		t.Fatalf("SearchBeliefs: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected promoted project belief, got %d", len(results))
	}
	if results[0].Belief.Confidence >= 0.95 {
		t.Fatalf("expected promotion confidence decay, got %f", results[0].Belief.Confidence)
	}
}

var _ llm.Provider = (*memoryTestProvider)(nil)
