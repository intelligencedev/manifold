package belief

import (
	"context"
	"math"
	"sort"
	"strings"
	"testing"
	"time"
)

type lifecycleTestStore struct {
	NoopStore
	scopes     map[string]Scope
	beliefs    map[string]Belief
	promotions []Promotion
}

func newLifecycleTestStore() *lifecycleTestStore {
	return &lifecycleTestStore{scopes: map[string]Scope{}, beliefs: map[string]Belief{}}
}

func (s *lifecycleTestStore) EnsureScope(_ context.Context, scope Scope) (Scope, error) {
	if scope.ID == "" {
		scope.ID = string(scope.Kind) + ":" + scope.Path
	}
	s.scopes[scope.ID] = scope
	return scope, nil
}

func (s *lifecycleTestStore) GetScopeByID(_ context.Context, tenantID int64, id string) (Scope, bool, error) {
	scope, ok := s.scopes[id]
	if !ok || scope.TenantID != tenantID {
		return Scope{}, false, nil
	}
	return scope, true, nil
}

func (s *lifecycleTestStore) GetScope(_ context.Context, tenantID int64, kind ScopeKind, path string) (Scope, bool, error) {
	for _, scope := range s.scopes {
		if scope.TenantID == tenantID && scope.Kind == kind && scope.Path == path {
			return scope, true, nil
		}
	}
	return Scope{}, false, nil
}

func (s *lifecycleTestStore) UpsertBelief(_ context.Context, item Belief) (Belief, error) {
	if item.StatementHash == "" {
		item.StatementHash = StatementHash(item.Statement)
	}
	if item.ID == "" {
		item.ID = item.ScopeID + ":" + item.StatementHash
	}
	if item.Status == "" {
		item.Status = BeliefStatusActive
	}
	item.UpdatedAt = time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = item.UpdatedAt
	}
	s.beliefs[item.ID] = item
	return item, nil
}

func (s *lifecycleTestStore) GetBelief(_ context.Context, tenantID int64, id string) (Belief, bool, error) {
	item, ok := s.beliefs[id]
	if !ok || item.TenantID != tenantID {
		return Belief{}, false, nil
	}
	return item, true, nil
}

func (s *lifecycleTestStore) SearchBeliefs(_ context.Context, query SearchQuery) ([]SearchResult, error) {
	scopeSet := map[string]bool{}
	for _, id := range query.ScopeIDs {
		scopeSet[id] = true
	}
	statusSet := map[BeliefStatus]bool{}
	for _, status := range query.Statuses {
		statusSet[status] = true
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	out := make([]SearchResult, 0)
	for _, item := range s.beliefs {
		if item.TenantID != query.TenantID {
			continue
		}
		if len(scopeSet) > 0 && !scopeSet[item.ScopeID] {
			continue
		}
		if len(statusSet) > 0 && !statusSet[item.Status] {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(item.Statement), needle) {
			continue
		}
		out = append(out, SearchResult{Belief: item, Score: item.Confidence})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[:query.Limit]
	}
	return out, nil
}

func (s *lifecycleTestStore) RecordPromotion(_ context.Context, promotion Promotion) (Promotion, error) {
	if promotion.ID == "" {
		promotion.ID = "promotion-1"
	}
	s.promotions = append(s.promotions, promotion)
	return promotion, nil
}

type lifecycleGraph struct {
	edges map[string][]string
}

func newLifecycleGraph() *lifecycleGraph { return &lifecycleGraph{edges: map[string][]string{}} }
func (g *lifecycleGraph) UpsertNode(context.Context, string, []string, map[string]any) error {
	return nil
}
func (g *lifecycleGraph) UpsertEdge(_ context.Context, srcID, rel, dstID string, _ map[string]any) error {
	g.edges[srcID+"/"+rel] = append(g.edges[srcID+"/"+rel], dstID)
	return nil
}
func (g *lifecycleGraph) Neighbors(_ context.Context, id, rel string) ([]string, error) {
	return append([]string(nil), g.edges[id+"/"+rel]...), nil
}

func TestPromoteObjectiveBeliefCreatesAuditAndDecaysConfidence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newLifecycleTestStore()
	graph := newLifecycleGraph()
	objective, _ := store.EnsureScope(ctx, Scope{ID: "objective-scope", TenantID: 7, Kind: ScopeKindObjective, Path: "project-1/objective-1"})
	project, _ := store.EnsureScope(ctx, Scope{ID: "project-scope", TenantID: 7, Kind: ScopeKindProject, Path: "project-1"})
	source, _ := store.UpsertBelief(ctx, Belief{ID: "belief-1", TenantID: 7, ScopeID: objective.ID, Statement: "Transit coordinates shared state.", StatementHash: "transit", Confidence: 0.9, EvidenceFor: 3, Status: BeliefStatusActive})

	result, err := (LifecycleService{Store: store, Graph: graph, Policy: PromotionPolicy{ConfidenceThreshold: 0.8, MinEvidenceFor: 2, ScopeWideningDecay: 0.8}}).Promote(ctx, PromotionRequest{TenantID: 7, BeliefID: source.ID, ToScope: project, Reason: "corroborated"})
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if result.Belief.ScopeID != project.ID || math.Abs(result.Belief.Confidence-0.72) > 0.000001 {
		t.Fatalf("unexpected promoted belief: %+v", result.Belief)
	}
	if len(store.promotions) != 1 || store.promotions[0].FromScope != objective.ID || store.promotions[0].ToScope != project.ID {
		t.Fatalf("expected promotion audit row, got %+v", store.promotions)
	}
	if got := graph.edges[source.ID+"/"+RelationPromotedTo]; len(got) != 1 || got[0] != result.Belief.ID {
		t.Fatalf("expected graph promotion edge, got %+v", got)
	}
}

func TestPromoteProjectToOrgRequiresManualApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newLifecycleTestStore()
	project, _ := store.EnsureScope(ctx, Scope{ID: "project-scope", TenantID: 7, Kind: ScopeKindProject, Path: "project-1"})
	org, _ := store.EnsureScope(ctx, Scope{ID: "org-scope", TenantID: 7, Kind: ScopeKindOrg, Path: "7"})
	item, _ := store.UpsertBelief(ctx, Belief{ID: "belief-1", TenantID: 7, ScopeID: project.ID, Statement: "Use project convention.", StatementHash: "convention", Confidence: 0.95, EvidenceFor: 5, Status: BeliefStatusActive})

	_, err := (LifecycleService{Store: store}).Promote(ctx, PromotionRequest{TenantID: 7, BeliefID: item.ID, ToScope: org, Reason: "too broad"})
	if err == nil {
		t.Fatal("expected project-to-org promotion to require manual approval")
	}
	if _, err := (LifecycleService{Store: store}).Promote(ctx, PromotionRequest{TenantID: 7, BeliefID: item.ID, ToScope: org, Reason: "manual", ManualApproval: true}); err != nil {
		t.Fatalf("expected manual project-to-org promotion, got %v", err)
	}
}

func TestDecayStaleRetractAndSupersede(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newLifecycleTestStore()
	scope, _ := store.EnsureScope(ctx, Scope{ID: "scope-1", TenantID: 7, Kind: ScopeKindProject, Path: "project-1"})
	oldObserved := time.Now().UTC().Add(-10 * 24 * time.Hour)
	expiresAt := time.Now().UTC().Add(-time.Hour)
	stale, _ := store.UpsertBelief(ctx, Belief{ID: "stale", TenantID: 7, ScopeID: scope.ID, Statement: "Stale belief.", StatementHash: "stale", Confidence: 0.8, EvidenceFor: 2, Status: BeliefStatusActive, LastObserved: &oldObserved})
	expired, _ := store.UpsertBelief(ctx, Belief{ID: "expired", TenantID: 7, ScopeID: scope.ID, Statement: "Expired belief.", StatementHash: "expired", Confidence: 0.8, EvidenceFor: 2, Status: BeliefStatusActive, ExpiresAt: &expiresAt})
	service := LifecycleService{Store: store, Policy: PromotionPolicy{StaleAfter: 24 * time.Hour, StaleConfidenceDecay: 0.5}}

	changed, err := service.DecayStale(ctx, SearchQuery{TenantID: 7, ScopeIDs: []string{scope.ID}, Limit: 10}, time.Now().UTC())
	if err != nil {
		t.Fatalf("DecayStale returned error: %v", err)
	}
	if len(changed) != 2 {
		t.Fatalf("expected stale and expired changes, got %d", len(changed))
	}
	updatedStale, _, _ := store.GetBelief(ctx, 7, stale.ID)
	if math.Abs(updatedStale.Confidence-0.4) > 0.000001 || updatedStale.Status != BeliefStatusActive {
		t.Fatalf("expected stale confidence decay without deletion, got %+v", updatedStale)
	}
	updatedExpired, _, _ := store.GetBelief(ctx, 7, expired.ID)
	if updatedExpired.Status != BeliefStatusRetracted {
		t.Fatalf("expected expired belief retracted, got %+v", updatedExpired)
	}

	retracted, err := service.Retract(ctx, 7, stale.ID, "wrong", nil)
	if err != nil {
		t.Fatalf("Retract returned error: %v", err)
	}
	if retracted.Status != BeliefStatusRetracted || retracted.Metadata["retractionReason"] != "wrong" {
		t.Fatalf("unexpected retracted belief %+v", retracted)
	}
	oldBelief, replacement, err := service.Supersede(ctx, 7, expired.ID, Belief{Statement: "Replacement belief.", Confidence: 0.9, EvidenceFor: 1}, "new evidence")
	if err != nil {
		t.Fatalf("Supersede returned error: %v", err)
	}
	if oldBelief.Status != BeliefStatusSuperseded || replacement.Status != BeliefStatusActive {
		t.Fatalf("unexpected supersession old=%+v replacement=%+v", oldBelief, replacement)
	}
}
