package decision_test

import (
	"context"
	"testing"

	"manifold/internal/agent/memory/belief"
	. "manifold/internal/agent/memory/decision"
	"manifold/internal/persistence/databases"
)

func TestServiceTransitionMatrix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name string
		from DecisionStatus
		to   DecisionStatus
		ok   bool
	}{
		{"proposed active", DecisionStatusProposed, DecisionStatusActive, true},
		{"proposed revoked", DecisionStatusProposed, DecisionStatusRevoked, true},
		{"proposed stale", DecisionStatusProposed, DecisionStatusStale, false},
		{"active stale", DecisionStatusActive, DecisionStatusStale, true},
		{"active superseded", DecisionStatusActive, DecisionStatusSuperseded, true},
		{"active revoked", DecisionStatusActive, DecisionStatusRevoked, true},
		{"stale active", DecisionStatusStale, DecisionStatusActive, true},
		{"stale superseded", DecisionStatusStale, DecisionStatusSuperseded, true},
		{"superseded active", DecisionStatusSuperseded, DecisionStatusActive, false},
		{"revoked active", DecisionStatusRevoked, DecisionStatusActive, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := databases.NewMemoryDecisionStore()
			service := Service{Store: store}
			item, err := service.CreateDecision(ctx, Decision{
				TenantID:  7,
				ScopeID:   "scope",
				Statement: "Use pgvector for memory retrieval.",
				Status:    tc.from,
			})
			if err != nil {
				t.Fatalf("CreateDecision: %v", err)
			}
			_, err = service.TransitionDecision(ctx, 7, item.ID, tc.to, TransitionRequest{Reason: "test"})
			if tc.ok && err != nil {
				t.Fatalf("expected transition to succeed: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected transition to fail")
			}
		})
	}
}

func TestServiceSupersedeLinksOldDecision(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := Service{Store: store}
	old, err := service.CreateDecision(ctx, Decision{TenantID: 7, ScopeID: "scope", Statement: "Use sqlite for memory retrieval.", Status: DecisionStatusActive})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	updated, replacement, err := service.Supersede(ctx, 7, old.ID, Decision{ScopeID: "scope", Statement: "Use pgvector for memory retrieval.", Status: DecisionStatusActive}, "better hybrid search", nil)
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	if updated.Status != DecisionStatusSuperseded || updated.SupersededBy != replacement.ID {
		t.Fatalf("old decision not superseded correctly: %+v replacement=%+v", updated, replacement)
	}
	transitions, err := store.ListTransitions(ctx, TransitionQuery{TenantID: 7, DecisionID: old.ID})
	if err != nil {
		t.Fatalf("ListTransitions: %v", err)
	}
	if len(transitions) < 2 || transitions[len(transitions)-1].TriggerKind != TriggerSupersession {
		t.Fatalf("expected supersession audit row, got %+v", transitions)
	}
}

func TestAcceptCandidateResolvesAssumptionHints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	decisionStore := databases.NewMemoryDecisionStore()
	beliefStore := databases.NewMemoryBeliefStore()
	scope, err := beliefStore.EnsureScope(ctx, belief.Scope{TenantID: 7, Kind: belief.ScopeKindProject, Path: "project", Label: "project"})
	if err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	seed, err := beliefStore.UpsertBelief(ctx, belief.Belief{
		TenantID:      7,
		ScopeID:       scope.ID,
		Statement:     "Postgres is already available for memory storage.",
		StatementHash: belief.StatementHash("Postgres is already available for memory storage."),
		Confidence:    0.95,
		Status:        belief.BeliefStatusActive,
	})
	if err != nil {
		t.Fatalf("UpsertBelief: %v", err)
	}
	candidate, err := decisionStore.RecordCandidate(ctx, Candidate{
		TenantID:         7,
		EpisodeID:        "episode",
		ScopeID:          scope.ID,
		Statement:        "Use pgvector for memory retrieval.",
		Rationale:        "It keeps vector search near Postgres-backed memory.",
		Confidence:       0.8,
		ReviewState:      ReviewStateNeedsReview,
		ValidationStatus: CandidateValidationQueued,
		AssumptionHints:  []string{"Postgres"},
	})
	if err != nil {
		t.Fatalf("RecordCandidate: %v", err)
	}
	service := Service{Store: decisionStore, Belief: beliefStore, Config: ServiceConfig{AssumptionSimilarityFloor: 0.1}}
	accepted, err := service.AcceptCandidate(ctx, 7, candidate.ID, nil)
	if err != nil {
		t.Fatalf("AcceptCandidate: %v", err)
	}
	links, err := decisionStore.ListAssumptions(ctx, AssumptionQuery{TenantID: 7, DecisionID: accepted.ID})
	if err != nil {
		t.Fatalf("ListAssumptions: %v", err)
	}
	if len(links) != 1 || links[0].BeliefID != seed.ID || links[0].BeliefStatementAtLink == "" {
		t.Fatalf("unexpected assumption links: %+v", links)
	}
}
