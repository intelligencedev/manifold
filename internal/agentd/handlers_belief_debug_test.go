package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/persistence/databases"
)

func TestDebugBeliefsHandlerSearchAndHistoryAreTenantScoped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := databases.NewBeliefStore(nil)
	tenantOneScope, err := store.EnsureScope(ctx, belief.Scope{TenantID: 1, Kind: belief.ScopeKindObjective, Path: "objective-a"})
	if err != nil {
		t.Fatalf("ensure tenant one scope: %v", err)
	}
	tenantTwoScope, err := store.EnsureScope(ctx, belief.Scope{TenantID: 2, Kind: belief.ScopeKindObjective, Path: "objective-b"})
	if err != nil {
		t.Fatalf("ensure tenant two scope: %v", err)
	}
	tenantOneBelief, err := store.UpsertBelief(ctx, belief.Belief{TenantID: 1, ScopeID: tenantOneScope.ID, Statement: "shared debug belief", StatementHash: "tenant-one", Confidence: 0.9, EvidenceFor: 2})
	if err != nil {
		t.Fatalf("upsert tenant one belief: %v", err)
	}
	_, err = store.UpsertBelief(ctx, belief.Belief{TenantID: 2, ScopeID: tenantTwoScope.ID, Statement: "shared debug belief", StatementHash: "tenant-two", Confidence: 0.9, EvidenceFor: 2})
	if err != nil {
		t.Fatalf("upsert tenant two belief: %v", err)
	}
	_, err = store.AddEvidence(ctx, belief.Evidence{TenantID: 1, BeliefID: tenantOneBelief.ID, SourceKind: belief.SourceKindEpisode, SourceID: "episode-1", Polarity: belief.EvidencePolarityFor, Weight: 1, Note: "tenant one evidence"})
	if err != nil {
		t.Fatalf("add tenant one evidence: %v", err)
	}
	_, err = store.RecordPromotion(ctx, belief.Promotion{TenantID: 1, BeliefID: tenantOneBelief.ID, FromScope: tenantOneScope.ID, ToScope: "project-scope", Reason: "tenant one promotion", ConfidenceBefore: 0.9, ConfidenceAfter: 0.76})
	if err != nil {
		t.Fatalf("record tenant one promotion: %v", err)
	}

	app := &app{cfg: &config.Config{}, mgr: &databases.Manager{Belief: store, Graph: databases.NewMemoryGraph()}}
	router := newRouter(app)

	searchReq := httptest.NewRequest(http.MethodGet, "/api/debug/beliefs/search?user_id=1&q=shared", nil)
	searchRec := httptest.NewRecorder()
	router.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", searchRec.Code, searchRec.Body.String())
	}
	var results []belief.SearchResult
	if err := json.Unmarshal(searchRec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode search results: %v", err)
	}
	if len(results) != 1 || results[0].Belief.ID != tenantOneBelief.ID {
		t.Fatalf("expected only tenant one belief, got %#v", results)
	}

	evidenceReq := httptest.NewRequest(http.MethodGet, "/api/debug/beliefs/evidence?user_id=2&belief_id="+tenantOneBelief.ID, nil)
	evidenceRec := httptest.NewRecorder()
	router.ServeHTTP(evidenceRec, evidenceReq)
	if evidenceRec.Code != http.StatusOK {
		t.Fatalf("evidence status = %d body=%s", evidenceRec.Code, evidenceRec.Body.String())
	}
	var evidenceItems []belief.Evidence
	if err := json.Unmarshal(evidenceRec.Body.Bytes(), &evidenceItems); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if len(evidenceItems) != 0 {
		t.Fatalf("expected no cross-tenant evidence, got %#v", evidenceItems)
	}

	promotionReq := httptest.NewRequest(http.MethodGet, "/api/debug/beliefs/promotions?user_id=1&belief_id="+tenantOneBelief.ID, nil)
	promotionRec := httptest.NewRecorder()
	router.ServeHTTP(promotionRec, promotionReq)
	if promotionRec.Code != http.StatusOK {
		t.Fatalf("promotions status = %d body=%s", promotionRec.Code, promotionRec.Body.String())
	}
	var promotionItems []belief.Promotion
	if err := json.Unmarshal(promotionRec.Body.Bytes(), &promotionItems); err != nil {
		t.Fatalf("decode promotions: %v", err)
	}
	if len(promotionItems) != 1 || promotionItems[0].BeliefID != tenantOneBelief.ID {
		t.Fatalf("expected tenant one promotion, got %#v", promotionItems)
	}
}

func TestBeliefsStableRoutesListAndReviewCandidates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := databases.NewBeliefStore(nil)
	scope, err := store.EnsureScope(ctx, belief.Scope{TenantID: 1, Kind: belief.ScopeKindObjective, Path: "project/objective"})
	if err != nil {
		t.Fatalf("ensure scope: %v", err)
	}
	episode, err := store.UpsertEpisode(ctx, belief.Episode{TenantID: 1, ScopeID: scope.ID, ProjectID: "project", ObjectiveID: "objective", StartedAt: scope.CreatedAt})
	if err != nil {
		t.Fatalf("upsert episode: %v", err)
	}
	candidate, err := store.RecordCandidate(ctx, belief.CandidateRecord{
		TenantID:         1,
		EpisodeID:        episode.ID,
		ScopeID:          scope.ID,
		Statement:        "Belief candidates are operator-reviewable.",
		StatementHash:    belief.StatementHash("Belief candidates are operator-reviewable."),
		Kind:             belief.BeliefKindFact,
		Enforcement:      belief.EnforcementPrompt,
		Polarity:         belief.EvidencePolarityFor,
		Confidence:       0.6,
		SourceQuality:    0.6,
		ReviewState:      belief.ReviewStateNeedsReview,
		EvidenceNote:     "candidate queue test",
		ValidationStatus: belief.CandidateValidationQueued,
	})
	if err != nil {
		t.Fatalf("record candidate: %v", err)
	}
	app := &app{cfg: &config.Config{}, mgr: &databases.Manager{Belief: store, Graph: databases.NewMemoryGraph()}}
	router := newRouter(app)

	listReq := httptest.NewRequest(http.MethodGet, "/api/beliefs/candidates?user_id=1&review_state=needs_review", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("candidate list status = %d body=%s", listRec.Code, listRec.Body.String())
	}
	var candidates []belief.CandidateRecord
	if err := json.Unmarshal(listRec.Body.Bytes(), &candidates); err != nil {
		t.Fatalf("decode candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != candidate.ID {
		t.Fatalf("expected queued candidate, got %#v", candidates)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/beliefs/candidates/"+candidate.ID+"/accept?user_id=1", nil)
	acceptRec := httptest.NewRecorder()
	router.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("candidate accept status = %d body=%s", acceptRec.Code, acceptRec.Body.String())
	}
	updated, ok, err := store.GetCandidate(ctx, 1, candidate.ID)
	if err != nil || !ok {
		t.Fatalf("get candidate ok=%v err=%v", ok, err)
	}
	if updated.ReviewState != belief.ReviewStateOperatorApproved || updated.AcceptedBeliefID == "" {
		t.Fatalf("expected approved candidate linked to belief, got %#v", updated)
	}
}

func TestDebugBeliefsHandlerRequiresAuthWhenEnabled(t *testing.T) {
	t.Parallel()

	app := &app{cfg: &config.Config{Auth: config.AuthConfig{Enabled: true}}, mgr: &databases.Manager{Belief: databases.NewBeliefStore(nil)}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/debug/beliefs/search", nil)
	newRouter(app).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestDebugBeliefsHandlerRetractUsesAuthenticatedTenant(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := databases.NewBeliefStore(nil)
	scope, err := store.EnsureScope(ctx, belief.Scope{TenantID: 42, Kind: belief.ScopeKindObjective, Path: "objective"})
	if err != nil {
		t.Fatalf("ensure scope: %v", err)
	}
	item, err := store.UpsertBelief(ctx, belief.Belief{TenantID: 42, ScopeID: scope.ID, Statement: "retract me", StatementHash: "retract-me", Confidence: 0.9, EvidenceFor: 2})
	if err != nil {
		t.Fatalf("upsert belief: %v", err)
	}
	app := &app{cfg: &config.Config{Auth: config.AuthConfig{Enabled: true}}, mgr: &databases.Manager{Belief: store, Graph: databases.NewMemoryGraph()}}

	body := bytes.NewBufferString(`{"reason":"operator cleanup"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/debug/beliefs/"+item.ID+"/retract", body)
	req = req.WithContext(auth.WithUser(req.Context(), &auth.User{ID: 42}))
	rec := httptest.NewRecorder()
	newRouter(app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	updated, ok, err := store.GetBelief(ctx, 42, item.ID)
	if err != nil || !ok {
		t.Fatalf("get updated belief ok=%v err=%v", ok, err)
	}
	if updated.Status != belief.BeliefStatusRetracted {
		t.Fatalf("status = %q, want retracted", updated.Status)
	}
}
