package archaeology

import (
	"context"
	"testing"

	"manifold/internal/agent/memory/artifact"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/persistence/databases"
)

func TestReconstructReportsStaleLoadBearingAssumption(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	decisionStore := databases.NewMemoryDecisionStore()
	beliefStore := databases.NewMemoryBeliefStore()
	artifactStore := databases.NewMemoryArtifactStore()
	scope, err := beliefStore.EnsureScope(ctx, belief.Scope{TenantID: 7, Kind: belief.ScopeKindProject, Path: "project", Label: "project"})
	if err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	b, err := beliefStore.UpsertBelief(ctx, belief.Belief{
		TenantID:      7,
		ScopeID:       scope.ID,
		Statement:     "Postgres is available.",
		StatementHash: belief.StatementHash("Postgres is available."),
		Confidence:    0.2,
		Status:        belief.BeliefStatusActive,
	})
	if err != nil {
		t.Fatalf("UpsertBelief: %v", err)
	}
	service := &decision.Service{Store: decisionStore, Belief: beliefStore}
	d, err := service.CreateDecision(ctx, decision.Decision{TenantID: 7, ScopeID: scope.ID, Statement: "Use pgvector for memory search.", Status: decision.DecisionStatusActive})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	_, err = decisionStore.AddAssumption(ctx, decision.AssumptionLink{TenantID: 7, DecisionID: d.ID, BeliefID: b.ID, Criticality: decision.CriticalityLoadBearing, BeliefStatementAtLink: b.Statement, BeliefConfidenceAtLink: 0.9})
	if err != nil {
		t.Fatalf("AddAssumption: %v", err)
	}
	art, err := artifactStore.UpsertArtifact(ctx, artifact.Artifact{TenantID: 7, Kind: artifact.ArtifactGitCommit, ExternalID: "abc", Title: "commit", Excerpt: "decision evidence"})
	if err != nil {
		t.Fatalf("UpsertArtifact: %v", err)
	}
	_, err = decisionStore.AddEvidence(ctx, decision.DecisionEvidence{TenantID: 7, DecisionID: d.ID, SourceKind: "artifact", SourceID: art.ID})
	if err != nil {
		t.Fatalf("AddEvidence: %v", err)
	}
	report, err := (&Service{Decisions: service, Beliefs: beliefStore, Artifacts: artifactStore}).Reconstruct(ctx, 7, "pgvector", ReconstructOptions{ScopeIDs: []string{scope.ID}})
	if err != nil {
		t.Fatalf("Reconstruct: %v", err)
	}
	if len(report.Decisions) != 1 {
		t.Fatalf("expected one dossier, got %+v", report)
	}
	if report.Decisions[0].Verdict != "stale" || report.Decisions[0].Evidence[0].Artifact == nil {
		t.Fatalf("unexpected dossier: %+v", report.Decisions[0])
	}
}
