package decision_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/memory/belief"
	. "manifold/internal/agent/memory/decision"
	"manifold/internal/persistence/databases"
)

type fakeScopeLookup struct {
	scopes map[string]belief.Scope
}

func (f fakeScopeLookup) GetScope(_ context.Context, _ int64, kind belief.ScopeKind, path string) (belief.Scope, bool, error) {
	scope, ok := f.scopes[string(kind)+":"+path]
	return scope, ok, nil
}

func seedDecision(t *testing.T, service *Service, d Decision) Decision {
	t.Helper()
	created, err := service.CreateDecision(context.Background(), d)
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	return created
}

func TestStoreRetrieverScopeWalkPrefersCloserScopes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store}

	objectiveScoped := seedDecision(t, service, Decision{
		TenantID:   7,
		ScopeID:    "objective-scope-uuid",
		Statement:  "We decided to keep the migration runner deterministic.",
		Status:     DecisionStatusActive,
		Confidence: 0.7,
	})
	projectScoped := seedDecision(t, service, Decision{
		TenantID:   7,
		ScopeID:    "project/proj-a/memory/archaeology",
		Statement:  "We decided to gate auto-activation by confidence.",
		Status:     DecisionStatusActive,
		Confidence: 0.7,
	})
	// Stale decisions must never enter the lane.
	seedDecision(t, service, Decision{
		TenantID:   7,
		ScopeID:    "project/proj-a/memory/archaeology",
		Statement:  "We decided to use a legacy approach that is now stale.",
		Status:     DecisionStatusStale,
		Confidence: 0.9,
	})
	// Other-tenant decisions must never leak.
	seedDecision(t, service, Decision{
		TenantID:   8,
		ScopeID:    "project/proj-a/memory/archaeology",
		Statement:  "We decided something in another tenant.",
		Status:     DecisionStatusActive,
		Confidence: 0.9,
	})

	retriever := NewStoreRetriever(store, fakeScopeLookup{scopes: map[string]belief.Scope{
		string(belief.ScopeKindObjective) + ":proj-a/obj-1": {ID: "objective-scope-uuid", Kind: belief.ScopeKindObjective, Path: "proj-a/obj-1"},
	}})
	results, err := retriever.Retrieve(ctx, RetrievalRequest{
		TenantID:    7,
		UserID:      7,
		ProjectID:   "proj-a",
		ObjectiveID: "obj-1",
		SessionID:   "sess-1",
		Query:       "migration runner",
		Limit:       5,
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %#v", len(results), results)
	}
	if results[0].Decision.ID != objectiveScoped.ID {
		t.Fatalf("expected objective-scoped decision ranked first, got %#v", results[0].Decision)
	}
	if results[1].Decision.ID != projectScoped.ID {
		t.Fatalf("expected project-prefix decision second, got %#v", results[1].Decision)
	}
}

func TestStoreRetrieverTenantWideLexicalFallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := databases.NewMemoryDecisionStore()
	service := &Service{Store: store}
	item := seedDecision(t, service, Decision{
		TenantID:   7,
		ScopeID:    "team/unrelated-scope",
		Statement:  "We decided to standardize observability dashboards.",
		Status:     DecisionStatusActive,
		Confidence: 0.8,
	})

	retriever := NewStoreRetriever(store, nil)
	results, err := retriever.Retrieve(ctx, RetrievalRequest{
		TenantID:  7,
		UserID:    7,
		ProjectID: "proj-a",
		Query:     "observability dashboards",
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(results) != 1 || results[0].Decision.ID != item.ID {
		t.Fatalf("expected tenant-wide lexical match, got %#v", results)
	}
}

func TestBuildPromptSectionRendersVerdictMarkersAndBudget(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	results := []SearchResult{
		{Decision: Decision{ID: "dec-1", ScopeID: "project/proj-a", Statement: "We decided to keep activation deliberate.", Status: DecisionStatusActive, ReviewState: ReviewStateOperatorApproved, Confidence: 0.9, DecidedAt: now}},
		{Decision: Decision{ID: "dec-2", ScopeID: "project/proj-a", Statement: "We decided to gate auto-activation.", Status: DecisionStatusActive, ReviewState: ReviewStateNeedsReview, Confidence: 0.8, DecidedAt: now}},
	}
	block := BuildPromptSection(results, PromptOptions{MaxDecisions: 5, MaxTokens: 600})
	if !strings.Contains(block.Text, "## Recorded Decisions") {
		t.Fatalf("missing header:\n%s", block.Text)
	}
	if !strings.Contains(block.Text, "id=dec-1") || !strings.Contains(block.Text, "id=dec-2") {
		t.Fatalf("missing decision ids:\n%s", block.Text)
	}
	if !strings.Contains(block.Text, "[verdict:holds]") {
		t.Fatalf("missing holds verdict marker:\n%s", block.Text)
	}
	if !strings.Contains(block.Text, "[verdict:needs_review]") {
		t.Fatalf("missing needs_review verdict marker:\n%s", block.Text)
	}
	if len(block.Selected) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(block.Selected))
	}

	capped := BuildPromptSection(results, PromptOptions{MaxDecisions: 1, MaxTokens: 600})
	if len(capped.Selected) != 1 || len(capped.Overflow) != 1 {
		t.Fatalf("expected MaxDecisions cap, selected=%d overflow=%d", len(capped.Selected), len(capped.Overflow))
	}

	tight := BuildPromptSection(results, PromptOptions{MaxDecisions: 5, MaxTokens: 1})
	if tight.Text != "" || len(tight.Selected) != 0 {
		t.Fatalf("expected empty section under impossible budget, got %#v", tight)
	}
}

func TestVerdictMarkerMirrorsLifecycle(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		decision Decision
		want     string
	}{
		"stale":      {Decision{Status: DecisionStatusStale}, "[verdict:stale]"},
		"superseded": {Decision{Status: DecisionStatusSuperseded}, "[verdict:superseded]"},
		"revoked":    {Decision{Status: DecisionStatusRevoked}, "[verdict:revoked]"},
		"review":     {Decision{Status: DecisionStatusActive, ReviewState: ReviewStateNeedsReview}, "[verdict:needs_review]"},
		"approved":   {Decision{Status: DecisionStatusActive, ReviewState: ReviewStateOperatorApproved}, "[verdict:holds]"},
		"auto":       {Decision{Status: DecisionStatusActive, ReviewState: ReviewStateAutoActive}, "[verdict:holds auto_active]"},
	}
	for name, tc := range cases {
		if got := VerdictMarker(tc.decision); got != tc.want {
			t.Fatalf("%s: VerdictMarker = %q, want %q", name, got, tc.want)
		}
	}
}
