package databases

import (
	"context"
	"testing"

	"manifold/internal/agent/memory/belief"
)

func TestMemoryBeliefStoreTenantAndScopeFiltering(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryBeliefStore()

	scopeA, err := store.EnsureScope(ctx, belief.Scope{
		TenantID: 1,
		Kind:     belief.ScopeKindProject,
		Path:     "project/a",
		Label:    "Project A",
	})
	if err != nil {
		t.Fatalf("EnsureScope tenant A: %v", err)
	}
	scopeB, err := store.EnsureScope(ctx, belief.Scope{
		TenantID: 2,
		Kind:     belief.ScopeKindProject,
		Path:     "project/a",
		Label:    "Project A in tenant B",
	})
	if err != nil {
		t.Fatalf("EnsureScope tenant B: %v", err)
	}

	if _, err := store.UpsertBelief(ctx, belief.Belief{
		TenantID:      1,
		ScopeID:       scopeA.ID,
		Statement:     "Project A uses Transit for shared working memory.",
		StatementHash: "tenant-a-transit",
		Confidence:    0.8,
	}); err != nil {
		t.Fatalf("UpsertBelief tenant A: %v", err)
	}
	if _, err := store.UpsertBelief(ctx, belief.Belief{
		TenantID:      2,
		ScopeID:       scopeB.ID,
		Statement:     "Project A uses a private tenant B convention.",
		StatementHash: "tenant-b-private",
		Confidence:    0.9,
	}); err != nil {
		t.Fatalf("UpsertBelief tenant B: %v", err)
	}

	results, err := store.SearchBeliefs(ctx, belief.SearchQuery{
		TenantID: 1,
		ScopeIDs: []string{scopeA.ID},
		Query:    "Transit",
		Statuses: []belief.BeliefStatus{belief.BeliefStatusActive},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchBeliefs: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Belief.TenantID != 1 || results[0].Belief.ScopeID != scopeA.ID {
		t.Fatalf("unexpected result: %+v", results[0].Belief)
	}

	if _, ok, err := store.GetScope(ctx, 1, belief.ScopeKindProject, "project/a"); err != nil || !ok {
		t.Fatalf("expected tenant A scope, ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.GetBelief(ctx, 2, results[0].Belief.ID); err != nil || ok {
		t.Fatalf("expected tenant B not to read tenant A belief, ok=%v err=%v", ok, err)
	}
}
