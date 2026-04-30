package belief

import (
	"context"
	"strings"
	"testing"
	"time"
)

type retrievalTestStore struct {
	NoopStore
	scopes  map[string]Scope
	beliefs map[string][]Belief
	queries []SearchQuery
}

func (s *retrievalTestStore) GetScope(_ context.Context, tenantID int64, kind ScopeKind, path string) (Scope, bool, error) {
	scope, ok := s.scopes[string(kind)+"/"+path]
	if !ok || scope.TenantID != tenantID {
		return Scope{}, false, nil
	}
	return scope, true, nil
}

func (s *retrievalTestStore) SearchBeliefs(_ context.Context, query SearchQuery) ([]SearchResult, error) {
	s.queries = append(s.queries, query)
	out := make([]SearchResult, 0)
	for _, scopeID := range query.ScopeIDs {
		for _, item := range s.beliefs[scopeID] {
			if item.TenantID != query.TenantID || item.Status == BeliefStatusRetracted {
				continue
			}
			if query.Query != "" && !strings.Contains(strings.ToLower(item.Statement), strings.ToLower(query.Query)) {
				continue
			}
			out = append(out, SearchResult{Belief: item, Score: item.Confidence})
		}
	}
	return out, nil
}

func TestStoreRetrieverScopeWalkRanksNarrowerBeliefs(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	store := &retrievalTestStore{
		scopes: map[string]Scope{
			"objective/project-1/objective-1": {ID: "scope-objective", TenantID: 7, Kind: ScopeKindObjective, Path: "project-1/objective-1"},
			"project/project-1":               {ID: "scope-project", TenantID: 7, Kind: ScopeKindProject, Path: "project-1"},
		},
		beliefs: map[string][]Belief{
			"scope-objective": {{ID: "objective-belief", TenantID: 7, ScopeID: "scope-objective", Statement: "Transit should store active project coordination state.", StatementHash: "objective", Confidence: 0.55, EvidenceFor: 1, Status: BeliefStatusActive, LastObserved: &now}},
			"scope-project":   {{ID: "project-belief", TenantID: 7, ScopeID: "scope-project", Statement: "Transit should store active project coordination state.", StatementHash: "project", Confidence: 0.95, EvidenceFor: 4, Status: BeliefStatusActive, LastObserved: &now}},
		},
	}

	results, err := NewStoreRetriever(store).Retrieve(context.Background(), RetrievalRequest{
		TenantID:    7,
		UserID:      7,
		ProjectID:   "project-1",
		ObjectiveID: "objective-1",
		Query:       "Transit",
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two results, got %d", len(results))
	}
	if results[0].Belief.ID != "objective-belief" {
		t.Fatalf("expected narrower objective belief first, got %q", results[0].Belief.ID)
	}
	if len(store.queries) != 2 {
		t.Fatalf("expected one storage query per resolved scope, got %d", len(store.queries))
	}
	for _, query := range store.queries {
		if query.TenantID != 7 || len(query.ScopeIDs) != 1 || query.ScopeIDs[0] == "" {
			t.Fatalf("expected tenant/scope constrained storage query, got %+v", query)
		}
	}
}

func TestBuildPromptSectionAppliesBudgets(t *testing.T) {
	t.Parallel()

	results := []SearchResult{
		{Belief: Belief{Statement: "First belief should be included.", Confidence: 0.8, EvidenceFor: 3}, Scope: Scope{Kind: ScopeKindObjective, Path: "project-1/objective-1"}},
		{Belief: Belief{Statement: "Second belief should overflow by item budget.", Confidence: 0.7, EvidenceFor: 2}, Scope: Scope{Kind: ScopeKindProject, Path: "project-1"}},
	}
	prompt := BuildPromptSection(results, PromptOptions{MaxBeliefs: 1, MaxTokens: 200})
	if !strings.Contains(prompt.Text, "Shared Belief Memory") || !strings.Contains(prompt.Text, "confidence 0.80") {
		t.Fatalf("expected formatted belief prompt, got %q", prompt.Text)
	}
	if len(prompt.Selected) != 1 || len(prompt.Overflow) != 1 {
		t.Fatalf("expected one selected and one overflow, got %d/%d", len(prompt.Selected), len(prompt.Overflow))
	}
}

func TestGraphEnrichedRetrieverSurfacesContradictionNeighbor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newLifecycleTestStore()
	graph := newLifecycleGraph()
	scope, _ := store.EnsureScope(ctx, Scope{ID: "scope-1", TenantID: 7, Kind: ScopeKindProject, Path: "project-1"})
	base, _ := store.UpsertBelief(ctx, Belief{ID: "base", TenantID: 7, ScopeID: scope.ID, Statement: "Use Transit for coordination.", StatementHash: "base", Confidence: 0.85, EvidenceFor: 3, Status: BeliefStatusActive})
	conflict, _ := store.UpsertBelief(ctx, Belief{ID: "conflict", TenantID: 7, ScopeID: scope.ID, Statement: "Avoid the shared-state backend for coordination in this project.", StatementHash: "conflict", Confidence: 0.45, EvidenceFor: 1, EvidenceAgainst: 2, Status: BeliefStatusActive})
	if err := graph.UpsertEdge(ctx, base.ID, RelationContradicts, conflict.ID, nil); err != nil {
		t.Fatalf("UpsertEdge: %v", err)
	}
	retriever := NewGraphEnrichedRetriever(store, graph, 1)
	results, err := retriever.Retrieve(ctx, RetrievalRequest{TenantID: 7, ProjectID: "project-1", Query: "Transit", Limit: 5})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}
	foundConflict := false
	for _, result := range results {
		if result.Belief.ID == conflict.ID && strings.Contains(result.Reason, RelationContradicts) {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Fatalf("expected graph contradiction neighbor in results, got %+v", results)
	}
}
