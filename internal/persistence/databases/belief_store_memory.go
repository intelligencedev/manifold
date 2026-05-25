package databases

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent/belief"

	"github.com/google/uuid"
)

// NewMemoryBeliefStore returns an in-memory belief store for local development and tests.
func NewMemoryBeliefStore() belief.Store {
	return &memBeliefStore{
		scopes:     map[string]belief.Scope{},
		scopeIndex: map[string]string{},
		episodes:   map[string]belief.Episode{},
		beliefs:    map[string]belief.Belief{},
		evidence:   map[string]belief.Evidence{},
		promotions: map[string]belief.Promotion{},
		candidates: map[string]belief.CandidateRecord{},
	}
}

type memBeliefStore struct {
	mu         sync.RWMutex
	scopes     map[string]belief.Scope
	scopeIndex map[string]string
	episodes   map[string]belief.Episode
	beliefs    map[string]belief.Belief
	evidence   map[string]belief.Evidence
	promotions map[string]belief.Promotion
	candidates map[string]belief.CandidateRecord
}

func (s *memBeliefStore) Init(context.Context) error { return nil }

func (s *memBeliefStore) EnsureScope(_ context.Context, scope belief.Scope) (belief.Scope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	scope.TenantID = normalizeTenantID(scope.TenantID)
	scope.Kind = normalizeScopeKind(scope.Kind)
	scope.Path = strings.TrimSpace(scope.Path)
	indexKey := beliefScopeIndexKey(scope.TenantID, scope.Kind, scope.Path)
	if id, ok := s.scopeIndex[indexKey]; ok {
		return cloneBeliefScope(s.scopes[id]), nil
	}

	now := time.Now().UTC()
	if strings.TrimSpace(scope.ID) == "" {
		scope.ID = uuid.NewString()
	}
	if scope.Label == "" {
		scope.Label = scope.Path
	}
	if scope.Metadata == nil {
		scope.Metadata = map[string]any{}
	}
	if scope.CreatedAt.IsZero() {
		scope.CreatedAt = now
	}
	scope.UpdatedAt = now
	s.scopes[scope.ID] = cloneBeliefScope(scope)
	s.scopeIndex[indexKey] = scope.ID
	return cloneBeliefScope(scope), nil
}

func (s *memBeliefStore) GetScope(_ context.Context, tenantID int64, kind belief.ScopeKind, path string) (belief.Scope, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.scopeIndex[beliefScopeIndexKey(normalizeTenantID(tenantID), normalizeScopeKind(kind), strings.TrimSpace(path))]
	if !ok {
		return belief.Scope{}, false, nil
	}
	return cloneBeliefScope(s.scopes[id]), true, nil
}

func (s *memBeliefStore) GetScopeByID(_ context.Context, tenantID int64, id string) (belief.Scope, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	scope, ok := s.scopes[strings.TrimSpace(id)]
	if !ok || scope.TenantID != normalizeTenantID(tenantID) {
		return belief.Scope{}, false, nil
	}
	return cloneBeliefScope(scope), true, nil
}

func (s *memBeliefStore) UpsertEpisode(_ context.Context, episode belief.Episode) (belief.Episode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	episode.TenantID = normalizeTenantID(episode.TenantID)
	if strings.TrimSpace(episode.ID) == "" {
		episode.ID = uuid.NewString()
	}
	if episode.StartedAt.IsZero() {
		episode.StartedAt = time.Now().UTC()
	}
	if episode.Outcome == "" {
		episode.Outcome = "unknown"
	}
	if episode.OutcomeSignal == "" {
		episode.OutcomeSignal = "implicit"
	}
	if episode.Metadata == nil {
		episode.Metadata = map[string]any{}
	}
	s.episodes[episode.ID] = cloneBeliefEpisode(episode)
	return cloneBeliefEpisode(episode), nil
}

func (s *memBeliefStore) GetEpisode(_ context.Context, tenantID int64, id string) (belief.Episode, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	episode, ok := s.episodes[strings.TrimSpace(id)]
	if !ok || episode.TenantID != normalizeTenantID(tenantID) {
		return belief.Episode{}, false, nil
	}
	return cloneBeliefEpisode(episode), true, nil
}

func (s *memBeliefStore) UpsertBelief(_ context.Context, item belief.Belief) (belief.Belief, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item.TenantID = normalizeTenantID(item.TenantID)
	item.Status = normalizeBeliefStatus(item.Status)
	item.Kind = belief.NormalizeBeliefKind(item.Kind)
	item.Enforcement = belief.NormalizeEnforcement(item.Enforcement)
	item.ReviewState = belief.NormalizeReviewState(item.ReviewState)
	now := time.Now().UTC()
	if strings.TrimSpace(item.ID) == "" {
		item.ID = uuid.NewString()
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	for id, existing := range s.beliefs {
		if existing.TenantID == item.TenantID && existing.ScopeID == item.ScopeID && existing.StatementHash == item.StatementHash && id != item.ID {
			item.ID = id
			item.CreatedAt = existing.CreatedAt
			break
		}
	}
	s.beliefs[item.ID] = cloneBelief(item)
	return cloneBelief(item), nil
}

func (s *memBeliefStore) GetBelief(_ context.Context, tenantID int64, id string) (belief.Belief, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.beliefs[strings.TrimSpace(id)]
	if !ok || item.TenantID != normalizeTenantID(tenantID) {
		return belief.Belief{}, false, nil
	}
	return cloneBelief(item), true, nil
}

func (s *memBeliefStore) SearchBeliefs(_ context.Context, query belief.SearchQuery) ([]belief.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	tenantID := normalizeTenantID(query.TenantID)
	scopeSet := make(map[string]bool, len(query.ScopeIDs))
	for _, scopeID := range query.ScopeIDs {
		if trimmed := strings.TrimSpace(scopeID); trimmed != "" {
			scopeSet[trimmed] = true
		}
	}
	statusSet := make(map[belief.BeliefStatus]bool, len(query.Statuses))
	for _, status := range query.Statuses {
		statusSet[normalizeBeliefStatus(status)] = true
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))

	out := make([]belief.SearchResult, 0, limit)
	for _, item := range s.beliefs {
		if item.TenantID != tenantID {
			continue
		}
		if len(scopeSet) > 0 && !scopeSet[item.ScopeID] {
			continue
		}
		if len(statusSet) > 0 && !statusSet[normalizeBeliefStatus(item.Status)] {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(item.Statement), needle) {
			continue
		}
		score := item.Confidence
		if needle != "" {
			score += 0.1
		}
		out = append(out, belief.SearchResult{Belief: cloneBelief(item), Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Belief.UpdatedAt.After(out[j].Belief.UpdatedAt)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memBeliefStore) AddEvidence(_ context.Context, evidence belief.Evidence) (belief.Evidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	evidence.TenantID = normalizeTenantID(evidence.TenantID)
	if strings.TrimSpace(evidence.ID) == "" {
		evidence.ID = uuid.NewString()
	}
	if evidence.SourceKind == "" {
		evidence.SourceKind = belief.SourceKindEpisode
	}
	if evidence.Polarity == "" {
		evidence.Polarity = belief.EvidencePolarityFor
	}
	if evidence.Weight == 0 {
		evidence.Weight = 1
	}
	if evidence.Metadata == nil {
		evidence.Metadata = map[string]any{}
	}
	if evidence.CreatedAt.IsZero() {
		evidence.CreatedAt = time.Now().UTC()
	}
	s.evidence[evidence.ID] = cloneBeliefEvidence(evidence)
	return cloneBeliefEvidence(evidence), nil
}

func (s *memBeliefStore) ListEvidence(_ context.Context, query belief.EvidenceQuery) ([]belief.Evidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]belief.Evidence, 0, limit)
	for _, evidence := range s.evidence {
		if evidence.TenantID != normalizeTenantID(query.TenantID) {
			continue
		}
		if strings.TrimSpace(query.BeliefID) != "" && evidence.BeliefID != strings.TrimSpace(query.BeliefID) {
			continue
		}
		if strings.TrimSpace(query.EpisodeID) != "" && evidence.EpisodeID != strings.TrimSpace(query.EpisodeID) {
			continue
		}
		if strings.TrimSpace(query.SourceID) != "" && evidence.SourceID != strings.TrimSpace(query.SourceID) {
			continue
		}
		if query.Source != "" && evidence.SourceKind != query.Source {
			continue
		}
		if query.Polarity != "" && evidence.Polarity != query.Polarity {
			continue
		}
		out = append(out, cloneBeliefEvidence(evidence))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memBeliefStore) RecordPromotion(_ context.Context, promotion belief.Promotion) (belief.Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	promotion.TenantID = normalizeTenantID(promotion.TenantID)
	if strings.TrimSpace(promotion.ID) == "" {
		promotion.ID = uuid.NewString()
	}
	if promotion.Metadata == nil {
		promotion.Metadata = map[string]any{}
	}
	if promotion.CreatedAt.IsZero() {
		promotion.CreatedAt = time.Now().UTC()
	}
	s.promotions[promotion.ID] = cloneBeliefPromotion(promotion)
	return cloneBeliefPromotion(promotion), nil
}

func (s *memBeliefStore) ListPromotions(_ context.Context, query belief.PromotionQuery) ([]belief.Promotion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]belief.Promotion, 0, limit)
	for _, promotion := range s.promotions {
		if promotion.TenantID != normalizeTenantID(query.TenantID) {
			continue
		}
		if strings.TrimSpace(query.BeliefID) != "" && promotion.BeliefID != strings.TrimSpace(query.BeliefID) {
			continue
		}
		if strings.TrimSpace(query.FromScope) != "" && promotion.FromScope != strings.TrimSpace(query.FromScope) {
			continue
		}
		if strings.TrimSpace(query.ToScope) != "" && promotion.ToScope != strings.TrimSpace(query.ToScope) {
			continue
		}
		out = append(out, cloneBeliefPromotion(promotion))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memBeliefStore) RecordCandidate(_ context.Context, candidate belief.CandidateRecord) (belief.CandidateRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	candidate.TenantID = normalizeTenantID(candidate.TenantID)
	candidate.Kind = belief.NormalizeBeliefKind(candidate.Kind)
	candidate.Enforcement = belief.NormalizeEnforcement(candidate.Enforcement)
	candidate.ReviewState = belief.NormalizeReviewState(candidate.ReviewState)
	candidate.ValidationStatus = belief.NormalizeCandidateValidationStatus(candidate.ValidationStatus)
	if strings.TrimSpace(candidate.ID) == "" {
		candidate.ID = uuid.NewString()
	}
	if strings.TrimSpace(candidate.StatementHash) == "" && strings.TrimSpace(candidate.Statement) != "" {
		candidate.StatementHash = belief.StatementHash(candidate.Statement)
	}
	if candidate.NormalizedPayload == nil {
		candidate.NormalizedPayload = map[string]any{}
	}
	if candidate.Metadata == nil {
		candidate.Metadata = map[string]any{}
	}
	now := time.Now().UTC()
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = now
	}
	candidate.UpdatedAt = now
	s.candidates[candidate.ID] = cloneBeliefCandidate(candidate)
	return cloneBeliefCandidate(candidate), nil
}

func (s *memBeliefStore) GetCandidate(_ context.Context, tenantID int64, id string) (belief.CandidateRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	candidate, ok := s.candidates[strings.TrimSpace(id)]
	if !ok || candidate.TenantID != normalizeTenantID(tenantID) {
		return belief.CandidateRecord{}, false, nil
	}
	return cloneBeliefCandidate(candidate), true, nil
}

func (s *memBeliefStore) ListCandidates(_ context.Context, query belief.CandidateQuery) ([]belief.CandidateRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]belief.CandidateRecord, 0, limit)
	for _, candidate := range s.candidates {
		if candidate.TenantID != normalizeTenantID(query.TenantID) {
			continue
		}
		if strings.TrimSpace(query.EpisodeID) != "" && candidate.EpisodeID != strings.TrimSpace(query.EpisodeID) {
			continue
		}
		if query.ReviewState != "" && candidate.ReviewState != belief.NormalizeReviewState(query.ReviewState) {
			continue
		}
		if query.ValidationStatus != "" && candidate.ValidationStatus != belief.NormalizeCandidateValidationStatus(query.ValidationStatus) {
			continue
		}
		out = append(out, cloneBeliefCandidate(candidate))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func beliefScopeIndexKey(tenantID int64, kind belief.ScopeKind, path string) string {
	return strings.Join([]string{fmt.Sprint(normalizeTenantID(tenantID)), string(normalizeScopeKind(kind)), strings.TrimSpace(path)}, "\x00")
}

func normalizeTenantID(tenantID int64) int64 { return tenantID }

func normalizeScopeKind(kind belief.ScopeKind) belief.ScopeKind {
	if kind == "" {
		return belief.ScopeKindSession
	}
	return kind
}

func normalizeBeliefStatus(status belief.BeliefStatus) belief.BeliefStatus {
	if status == "" {
		return belief.BeliefStatusActive
	}
	return status
}

func cloneBeliefScope(scope belief.Scope) belief.Scope {
	scope.Metadata = cloneAnyMap(scope.Metadata)
	return scope
}

func cloneBeliefEpisode(episode belief.Episode) belief.Episode {
	episode.Metadata = cloneAnyMap(episode.Metadata)
	if episode.EndedAt != nil {
		endedAt := *episode.EndedAt
		episode.EndedAt = &endedAt
	}
	return episode
}

func cloneBelief(item belief.Belief) belief.Belief {
	item.Metadata = cloneAnyMap(item.Metadata)
	item.Embedding = append([]float32(nil), item.Embedding...)
	if item.LastObserved != nil {
		lastObserved := *item.LastObserved
		item.LastObserved = &lastObserved
	}
	if item.ExpiresAt != nil {
		expiresAt := *item.ExpiresAt
		item.ExpiresAt = &expiresAt
	}
	return item
}

func cloneBeliefEvidence(evidence belief.Evidence) belief.Evidence {
	evidence.Metadata = cloneAnyMap(evidence.Metadata)
	return evidence
}

func cloneBeliefPromotion(promotion belief.Promotion) belief.Promotion {
	promotion.Metadata = cloneAnyMap(promotion.Metadata)
	if promotion.ActorUserID != nil {
		actorUserID := *promotion.ActorUserID
		promotion.ActorUserID = &actorUserID
	}
	return promotion
}

func cloneBeliefCandidate(candidate belief.CandidateRecord) belief.CandidateRecord {
	candidate.NormalizedPayload = cloneAnyMap(candidate.NormalizedPayload)
	candidate.Metadata = cloneAnyMap(candidate.Metadata)
	return candidate
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}
