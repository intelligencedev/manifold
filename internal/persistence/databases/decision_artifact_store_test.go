package databases

import (
	"context"
	"testing"

	"manifold/internal/agent/memory/artifact"
	"manifold/internal/agent/memory/decision"
)

func TestSQLiteDecisionStoreParity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewSQLiteDecisionStore(openTestSQLite(t))
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	item, err := store.UpsertDecision(ctx, decision.Decision{
		TenantID:  7,
		ScopeID:   "scope",
		Statement: "Use pgvector for memory search.",
		Rationale: "It keeps vector retrieval near the existing database.",
		Status:    decision.DecisionStatusActive,
	})
	if err != nil {
		t.Fatalf("UpsertDecision: %v", err)
	}
	if _, err := store.AddAssumption(ctx, decision.AssumptionLink{
		TenantID:               7,
		DecisionID:             item.ID,
		BeliefID:               "belief-1",
		Criticality:            decision.CriticalityLoadBearing,
		BeliefStatementAtLink:  "Postgres is available.",
		BeliefConfidenceAtLink: 0.9,
	}); err != nil {
		t.Fatalf("AddAssumption: %v", err)
	}
	if _, err := store.AddAlternative(ctx, decision.Alternative{TenantID: 7, DecisionID: item.ID, Statement: "Use Qdrant.", RejectionReason: "Extra service to operate."}); err != nil {
		t.Fatalf("AddAlternative: %v", err)
	}
	if _, err := store.AddEvidence(ctx, decision.DecisionEvidence{TenantID: 7, DecisionID: item.ID, SourceKind: "artifact", SourceID: "artifact-1"}); err != nil {
		t.Fatalf("AddEvidence: %v", err)
	}
	if _, err := store.RecordTransition(ctx, decision.Transition{TenantID: 7, DecisionID: item.ID, ToStatus: decision.DecisionStatusActive}); err != nil {
		t.Fatalf("RecordTransition: %v", err)
	}
	got, ok, err := store.GetDecision(ctx, 7, item.ID)
	if err != nil || !ok {
		t.Fatalf("GetDecision ok=%v err=%v", ok, err)
	}
	if got.StatementHash == "" || got.Statement != item.Statement {
		t.Fatalf("unexpected decision: %+v", got)
	}
	results, err := store.SearchDecisions(ctx, decision.SearchQuery{TenantID: 7, ScopeIDs: []string{"scope"}, Query: "pgvector", Statuses: []decision.DecisionStatus{decision.DecisionStatusActive}})
	if err != nil || len(results) != 1 {
		t.Fatalf("SearchDecisions len=%d err=%v", len(results), err)
	}
	if assumptions, _ := store.ListAssumptions(ctx, decision.AssumptionQuery{TenantID: 7, DecisionID: item.ID}); len(assumptions) != 1 {
		t.Fatalf("expected assumptions")
	}
	if alternatives, _ := store.ListAlternatives(ctx, 7, item.ID); len(alternatives) != 1 {
		t.Fatalf("expected alternatives")
	}
	if evidence, _ := store.ListEvidence(ctx, decision.EvidenceQuery{TenantID: 7, DecisionID: item.ID}); len(evidence) != 1 {
		t.Fatalf("expected evidence")
	}
	if transitions, _ := store.ListTransitions(ctx, decision.TransitionQuery{TenantID: 7, DecisionID: item.ID}); len(transitions) != 1 {
		t.Fatalf("expected transitions")
	}
}

func TestSQLiteArtifactStoreParity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewSQLiteArtifactStore(openTestSQLite(t))
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	first, err := store.UpsertArtifact(ctx, artifact.Artifact{
		TenantID:   7,
		Kind:       artifact.ArtifactGitCommit,
		ExternalID: "abc",
		Title:      "Adopt pgvector",
		Excerpt:    "Decision commit",
	})
	if err != nil {
		t.Fatalf("UpsertArtifact first: %v", err)
	}
	second, err := store.UpsertArtifact(ctx, artifact.Artifact{
		TenantID:   7,
		Kind:       artifact.ArtifactGitCommit,
		ExternalID: "abc",
		Title:      "Adopt pgvector",
		Excerpt:    "Decision commit",
	})
	if err != nil {
		t.Fatalf("UpsertArtifact second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected idempotent upsert, first=%+v second=%+v", first, second)
	}
	byExternal, err := store.FindByExternalID(ctx, 7, artifact.ArtifactGitCommit, "abc")
	if err != nil || len(byExternal) != 1 {
		t.Fatalf("FindByExternalID len=%d err=%v", len(byExternal), err)
	}
	results, err := store.SearchArtifacts(ctx, artifact.SearchQuery{TenantID: 7, Query: "pgvector"})
	if err != nil || len(results) != 1 {
		t.Fatalf("SearchArtifacts len=%d err=%v", len(results), err)
	}
}
