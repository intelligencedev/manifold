package belief

import "context"

// Store persists scopes, episode envelopes, beliefs, evidence, and promotions.
type Store interface {
	Init(ctx context.Context) error
	EnsureScope(ctx context.Context, scope Scope) (Scope, error)
	GetScope(ctx context.Context, tenantID int64, kind ScopeKind, path string) (Scope, bool, error)
	GetScopeByID(ctx context.Context, tenantID int64, id string) (Scope, bool, error)
	UpsertEpisode(ctx context.Context, episode Episode) (Episode, error)
	GetEpisode(ctx context.Context, tenantID int64, id string) (Episode, bool, error)
	UpsertBelief(ctx context.Context, belief Belief) (Belief, error)
	GetBelief(ctx context.Context, tenantID int64, id string) (Belief, bool, error)
	SearchBeliefs(ctx context.Context, query SearchQuery) ([]SearchResult, error)
	AddEvidence(ctx context.Context, evidence Evidence) (Evidence, error)
	ListEvidence(ctx context.Context, query EvidenceQuery) ([]Evidence, error)
	RecordPromotion(ctx context.Context, promotion Promotion) (Promotion, error)
	ListPromotions(ctx context.Context, query PromotionQuery) ([]Promotion, error)
}

// NoopStore is a disabled belief store implementation.
type NoopStore struct{}

func (NoopStore) Init(context.Context) error { return nil }
func (NoopStore) EnsureScope(_ context.Context, scope Scope) (Scope, error) {
	return scope, nil
}
func (NoopStore) GetScope(context.Context, int64, ScopeKind, string) (Scope, bool, error) {
	return Scope{}, false, nil
}
func (NoopStore) GetScopeByID(context.Context, int64, string) (Scope, bool, error) {
	return Scope{}, false, nil
}
func (NoopStore) UpsertEpisode(_ context.Context, episode Episode) (Episode, error) {
	return episode, nil
}
func (NoopStore) GetEpisode(context.Context, int64, string) (Episode, bool, error) {
	return Episode{}, false, nil
}
func (NoopStore) UpsertBelief(_ context.Context, belief Belief) (Belief, error) {
	return belief, nil
}
func (NoopStore) GetBelief(context.Context, int64, string) (Belief, bool, error) {
	return Belief{}, false, nil
}
func (NoopStore) SearchBeliefs(context.Context, SearchQuery) ([]SearchResult, error) {
	return nil, nil
}
func (NoopStore) AddEvidence(_ context.Context, evidence Evidence) (Evidence, error) {
	return evidence, nil
}
func (NoopStore) ListEvidence(context.Context, EvidenceQuery) ([]Evidence, error) {
	return nil, nil
}
func (NoopStore) RecordPromotion(_ context.Context, promotion Promotion) (Promotion, error) {
	return promotion, nil
}
func (NoopStore) ListPromotions(context.Context, PromotionQuery) ([]Promotion, error) {
	return nil, nil
}
