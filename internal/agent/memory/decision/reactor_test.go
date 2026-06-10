package decision_test

import (
	"context"
	"testing"

	"manifold/internal/agent/memory/belief"
	. "manifold/internal/agent/memory/decision"
	"manifold/internal/persistence/databases"
)

func TestReactorMarksLoadBearingDecisionStale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store}
	item, err := service.CreateDecision(ctx, Decision{TenantID: 7, ScopeID: "scope", Statement: "Use pgvector for memory search.", Status: DecisionStatusActive})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	_, err = store.AddAssumption(ctx, AssumptionLink{
		TenantID:               7,
		DecisionID:             item.ID,
		BeliefID:               "belief-1",
		Criticality:            CriticalityLoadBearing,
		BeliefStatementAtLink:  "Postgres is available.",
		BeliefConfidenceAtLink: 0.9,
	})
	if err != nil {
		t.Fatalf("AddAssumption: %v", err)
	}
	reactor := Reactor{Decisions: service}
	reactor.OnBeliefChanged(ctx, belief.ChangeEvent{
		TenantID: 7,
		BeliefID: "belief-1",
		Kind:     belief.ChangeStatus,
		Before:   belief.Belief{Status: belief.BeliefStatusActive},
		After:    belief.Belief{Status: belief.BeliefStatusRetracted},
	})
	got, _, err := store.GetDecision(ctx, 7, item.ID)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.Status != DecisionStatusStale || got.ReviewState != ReviewStateNeedsReview || got.StaleReason == "" {
		t.Fatalf("expected stale decision with reason, got %+v", got)
	}
}

func TestReactorFlagsSupportingDecisionForReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store}
	item, err := service.CreateDecision(ctx, Decision{TenantID: 7, ScopeID: "scope", Statement: "Use pgvector for memory search.", Status: DecisionStatusActive})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	_, err = store.AddAssumption(ctx, AssumptionLink{TenantID: 7, DecisionID: item.ID, BeliefID: "belief-1", Criticality: CriticalitySupporting, BeliefStatementAtLink: "Postgres is available.", BeliefConfidenceAtLink: 0.9})
	if err != nil {
		t.Fatalf("AddAssumption: %v", err)
	}
	reactor := Reactor{Decisions: service}
	reactor.OnBeliefChanged(ctx, belief.ChangeEvent{
		TenantID: 7,
		BeliefID: "belief-1",
		Kind:     belief.ChangeStatus,
		Before:   belief.Belief{Status: belief.BeliefStatusActive},
		After:    belief.Belief{Status: belief.BeliefStatusSuperseded},
	})
	got, _, err := store.GetDecision(ctx, 7, item.ID)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.Status != DecisionStatusActive || got.ReviewState != ReviewStateNeedsReview {
		t.Fatalf("expected active decision flagged for review, got %+v", got)
	}
}
