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

func TestSQLiteBeliefStoreParity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewSQLiteBeliefStore(openTestSQLite(t))
	if err := store.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	scope, err := store.EnsureScope(ctx, belief.Scope{
		TenantID: 1,
		Kind:     belief.ScopeKindProject,
		Path:     "project/sqlite",
		Label:    "SQLite Project",
		Metadata: map[string]any{"region": "local"},
	})
	if err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	episode, err := store.UpsertEpisode(ctx, belief.Episode{
		TenantID:    1,
		ScopeID:     scope.ID,
		ProjectID:   "project/sqlite",
		ObjectiveID: "objective-1",
		SessionID:   "session-1",
		UserID:      7,
	})
	if err != nil {
		t.Fatalf("UpsertEpisode: %v", err)
	}

	item, err := store.UpsertBelief(ctx, belief.Belief{
		TenantID:      1,
		ScopeID:       scope.ID,
		Statement:     "SQLite belief memory supports local durable recall.",
		StatementHash: "sqlite-recall",
		Confidence:    0.83,
		Metadata:      map[string]any{"source": "test"},
	})
	if err != nil {
		t.Fatalf("UpsertBelief: %v", err)
	}
	results, err := store.SearchBeliefs(ctx, belief.SearchQuery{
		TenantID: 1,
		ScopeIDs: []string{scope.ID},
		Query:    "durable recall",
		Statuses: []belief.BeliefStatus{belief.BeliefStatusActive},
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("SearchBeliefs: %v", err)
	}
	if len(results) != 1 || results[0].Belief.ID != item.ID {
		t.Fatalf("unexpected search results: %+v", results)
	}

	evidence, err := store.AddEvidence(ctx, belief.Evidence{
		TenantID:  1,
		BeliefID:  item.ID,
		EpisodeID: episode.ID,
		SourceID:  "source-1",
		Note:      "observed in test",
	})
	if err != nil {
		t.Fatalf("AddEvidence: %v", err)
	}
	evidenceList, err := store.ListEvidence(ctx, belief.EvidenceQuery{TenantID: 1, BeliefID: item.ID})
	if err != nil {
		t.Fatalf("ListEvidence: %v", err)
	}
	if len(evidenceList) != 1 || evidenceList[0].ID != evidence.ID {
		t.Fatalf("unexpected evidence list: %+v", evidenceList)
	}

	promotion, err := store.RecordPromotion(ctx, belief.Promotion{
		TenantID:         1,
		BeliefID:         item.ID,
		FromScope:        scope.ID,
		ToScope:          scope.ID,
		Reason:           "same-scope audit",
		ConfidenceBefore: 0.7,
		ConfidenceAfter:  0.83,
	})
	if err != nil {
		t.Fatalf("RecordPromotion: %v", err)
	}
	promotions, err := store.ListPromotions(ctx, belief.PromotionQuery{TenantID: 1, BeliefID: item.ID})
	if err != nil {
		t.Fatalf("ListPromotions: %v", err)
	}
	if len(promotions) != 1 || promotions[0].ID != promotion.ID {
		t.Fatalf("unexpected promotions: %+v", promotions)
	}

	candidate, err := store.RecordCandidate(ctx, belief.CandidateRecord{
		TenantID:         1,
		EpisodeID:        episode.ID,
		ScopeID:          scope.ID,
		Statement:        "Candidates persist in SQLite.",
		Confidence:       0.6,
		ReviewState:      belief.ReviewStateNeedsReview,
		ValidationStatus: belief.CandidateValidationQueued,
	})
	if err != nil {
		t.Fatalf("RecordCandidate: %v", err)
	}
	gotCandidate, ok, err := store.GetCandidate(ctx, 1, candidate.ID)
	if err != nil || !ok {
		t.Fatalf("GetCandidate ok=%v err=%v", ok, err)
	}
	if gotCandidate.StatementHash == "" {
		t.Fatalf("expected statement hash to be populated: %+v", gotCandidate)
	}
	candidates, err := store.ListCandidates(ctx, belief.CandidateQuery{
		TenantID:         1,
		EpisodeID:        episode.ID,
		ReviewState:      belief.ReviewStateNeedsReview,
		ValidationStatus: belief.CandidateValidationQueued,
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != candidate.ID {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}
