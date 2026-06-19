package databases

import (
	"context"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"manifold/internal/agent/memory/decision"

	"github.com/google/uuid"
)

// NewMemoryDecisionStore returns an in-memory decision store for local development and tests.
func NewMemoryDecisionStore() decision.Store {
	return &memDecisionStore{
		decisions:    map[string]decision.Decision{},
		assumptions:  map[string]decision.AssumptionLink{},
		alternatives: map[string]decision.Alternative{},
		evidence:     map[string]decision.DecisionEvidence{},
		transitions:  map[string]decision.Transition{},
		candidates:   map[string]decision.Candidate{},
	}
}

type memDecisionStore struct {
	mu           sync.RWMutex
	decisions    map[string]decision.Decision
	assumptions  map[string]decision.AssumptionLink
	alternatives map[string]decision.Alternative
	evidence     map[string]decision.DecisionEvidence
	transitions  map[string]decision.Transition
	candidates   map[string]decision.Candidate
}

func (s *memDecisionStore) Init(context.Context) error { return nil }

func (s *memDecisionStore) UpsertDecision(_ context.Context, item decision.Decision) (decision.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item = prepareDecision(item)
	for id, existing := range s.decisions {
		if existing.TenantID == item.TenantID && existing.ScopeID == item.ScopeID && existing.StatementHash == item.StatementHash && id != item.ID {
			item.ID = id
			item.CreatedAt = existing.CreatedAt
			break
		}
	}
	s.decisions[item.ID] = cloneDecision(item)
	return cloneDecision(item), nil
}

func (s *memDecisionStore) GetDecision(_ context.Context, tenantID int64, id string) (decision.Decision, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.decisions[strings.TrimSpace(id)]
	if !ok || item.TenantID != normalizeTenantID(tenantID) {
		return decision.Decision{}, false, nil
	}
	return cloneDecision(item), true, nil
}

func (s *memDecisionStore) SearchDecisions(_ context.Context, query decision.SearchQuery) ([]decision.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	tenantID := normalizeTenantID(query.TenantID)
	statusSet := map[decision.DecisionStatus]bool{}
	for _, status := range query.Statuses {
		statusSet[decision.NormalizeDecisionStatus(status)] = true
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	out := make([]decision.SearchResult, 0, limit)
	for _, item := range s.decisions {
		if item.TenantID != tenantID {
			continue
		}
		if !query.MatchesScope(item.ScopeID) {
			continue
		}
		if len(statusSet) > 0 && !statusSet[decision.NormalizeDecisionStatus(item.Status)] {
			continue
		}
		text := strings.ToLower(item.Title + " " + item.Statement + " " + item.Rationale)
		if needle != "" && !strings.Contains(text, needle) && item.StatementHash != needle {
			continue
		}
		score := item.Confidence
		if needle != "" {
			score += 0.1
		}
		out = append(out, decision.SearchResult{Decision: cloneDecision(item), Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Decision.UpdatedAt.After(out[j].Decision.UpdatedAt)
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memDecisionStore) AddAssumption(_ context.Context, link decision.AssumptionLink) (decision.AssumptionLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	link.TenantID = normalizeTenantID(link.TenantID)
	if strings.TrimSpace(link.ID) == "" {
		link.ID = uuid.NewString()
	}
	link.Criticality = decision.NormalizeAssumptionCriticality(link.Criticality)
	if link.Metadata == nil {
		link.Metadata = map[string]any{}
	}
	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now().UTC()
	}
	s.assumptions[link.ID] = cloneAssumption(link)
	return cloneAssumption(link), nil
}

func (s *memDecisionStore) ListAssumptions(_ context.Context, query decision.AssumptionQuery) ([]decision.AssumptionLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]decision.AssumptionLink, 0, limit)
	for _, link := range s.assumptions {
		if link.TenantID != normalizeTenantID(query.TenantID) {
			continue
		}
		if query.DecisionID != "" && link.DecisionID != strings.TrimSpace(query.DecisionID) {
			continue
		}
		if query.BeliefID != "" && link.BeliefID != strings.TrimSpace(query.BeliefID) {
			continue
		}
		out = append(out, cloneAssumption(link))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memDecisionStore) AddAlternative(_ context.Context, alt decision.Alternative) (decision.Alternative, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	alt.TenantID = normalizeTenantID(alt.TenantID)
	if strings.TrimSpace(alt.ID) == "" {
		alt.ID = uuid.NewString()
	}
	alt.Statement = decision.NormalizeStatement(alt.Statement)
	if alt.Metadata == nil {
		alt.Metadata = map[string]any{}
	}
	if alt.CreatedAt.IsZero() {
		alt.CreatedAt = time.Now().UTC()
	}
	s.alternatives[alt.ID] = cloneAlternative(alt)
	return cloneAlternative(alt), nil
}

func (s *memDecisionStore) ListAlternatives(_ context.Context, tenantID int64, decisionID string) ([]decision.Alternative, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []decision.Alternative{}
	for _, alt := range s.alternatives {
		if alt.TenantID != normalizeTenantID(tenantID) || alt.DecisionID != strings.TrimSpace(decisionID) {
			continue
		}
		out = append(out, cloneAlternative(alt))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *memDecisionStore) AddEvidence(_ context.Context, ev decision.DecisionEvidence) (decision.DecisionEvidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev.TenantID = normalizeTenantID(ev.TenantID)
	if strings.TrimSpace(ev.ID) == "" {
		ev.ID = uuid.NewString()
	}
	ev.SourceKind = strings.TrimSpace(ev.SourceKind)
	ev.SourceID = strings.TrimSpace(ev.SourceID)
	ev.Polarity = decision.NormalizeEvidencePolarity(ev.Polarity)
	if ev.Weight == 0 {
		ev.Weight = 1
	}
	if ev.Metadata == nil {
		ev.Metadata = map[string]any{}
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	s.evidence[ev.ID] = cloneDecisionEvidence(ev)
	return cloneDecisionEvidence(ev), nil
}

func (s *memDecisionStore) ListEvidence(_ context.Context, query decision.EvidenceQuery) ([]decision.DecisionEvidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]decision.DecisionEvidence, 0, limit)
	for _, ev := range s.evidence {
		if ev.TenantID != normalizeTenantID(query.TenantID) {
			continue
		}
		if query.DecisionID != "" && ev.DecisionID != strings.TrimSpace(query.DecisionID) {
			continue
		}
		if query.SourceKind != "" && ev.SourceKind != strings.TrimSpace(query.SourceKind) {
			continue
		}
		if query.SourceID != "" && ev.SourceID != strings.TrimSpace(query.SourceID) {
			continue
		}
		if query.Polarity != "" && ev.Polarity != decision.NormalizeEvidencePolarity(query.Polarity) {
			continue
		}
		out = append(out, cloneDecisionEvidence(ev))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memDecisionStore) RecordTransition(_ context.Context, tr decision.Transition) (decision.Transition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tr.TenantID = normalizeTenantID(tr.TenantID)
	if strings.TrimSpace(tr.ID) == "" {
		tr.ID = uuid.NewString()
	}
	if tr.CreatedAt.IsZero() {
		tr.CreatedAt = time.Now().UTC()
	}
	s.transitions[tr.ID] = cloneTransition(tr)
	return cloneTransition(tr), nil
}

func (s *memDecisionStore) ListTransitions(_ context.Context, query decision.TransitionQuery) ([]decision.Transition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]decision.Transition, 0, limit)
	for _, tr := range s.transitions {
		if tr.TenantID != normalizeTenantID(query.TenantID) {
			continue
		}
		if query.DecisionID != "" && tr.DecisionID != strings.TrimSpace(query.DecisionID) {
			continue
		}
		out = append(out, cloneTransition(tr))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memDecisionStore) RecordCandidate(_ context.Context, c decision.Candidate) (decision.Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.TenantID = normalizeTenantID(c.TenantID)
	if strings.TrimSpace(c.ID) == "" {
		c.ID = uuid.NewString()
	}
	c.Statement = decision.NormalizeStatement(c.Statement)
	if strings.TrimSpace(c.StatementHash) == "" {
		c.StatementHash = decision.StatementHash(c.Statement)
	}
	c.ReviewState = decision.NormalizeReviewState(c.ReviewState)
	c.ValidationStatus = decision.NormalizeCandidateValidationStatus(c.ValidationStatus)
	c.Confidence = clampDecisionConfidence(c.Confidence)
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Metadata == nil {
		c.Metadata = map[string]any{}
	}
	s.candidates[c.ID] = cloneCandidate(c)
	return cloneCandidate(c), nil
}

func (s *memDecisionStore) GetCandidate(_ context.Context, tenantID int64, id string) (decision.Candidate, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.candidates[strings.TrimSpace(id)]
	if !ok || c.TenantID != normalizeTenantID(tenantID) {
		return decision.Candidate{}, false, nil
	}
	return cloneCandidate(c), true, nil
}

func (s *memDecisionStore) ListCandidates(_ context.Context, query decision.CandidateQuery) ([]decision.Candidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	out := make([]decision.Candidate, 0, limit)
	for _, c := range s.candidates {
		if c.TenantID != normalizeTenantID(query.TenantID) {
			continue
		}
		if query.EpisodeID != "" && c.EpisodeID != strings.TrimSpace(query.EpisodeID) {
			continue
		}
		if query.ReviewState != "" && c.ReviewState != decision.NormalizeReviewState(query.ReviewState) {
			continue
		}
		if query.ValidationStatus != "" && c.ValidationStatus != decision.NormalizeCandidateValidationStatus(query.ValidationStatus) {
			continue
		}
		out = append(out, cloneCandidate(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func prepareDecision(item decision.Decision) decision.Decision {
	now := time.Now().UTC()
	item.TenantID = normalizeTenantID(item.TenantID)
	if strings.TrimSpace(item.ID) == "" {
		item.ID = uuid.NewString()
	}
	item.Statement = decision.NormalizeStatement(item.Statement)
	if strings.TrimSpace(item.StatementHash) == "" {
		item.StatementHash = decision.StatementHash(item.Statement)
	}
	item.Status = decision.NormalizeDecisionStatus(item.Status)
	item.ReviewState = decision.NormalizeReviewState(item.ReviewState)
	item.Confidence = clampDecisionConfidence(item.Confidence)
	if item.Title == "" {
		item.Title = item.Statement
	}
	if item.DecidedAt.IsZero() {
		item.DecidedAt = now
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item
}

func cloneDecision(item decision.Decision) decision.Decision {
	item.Embedding = append([]float32(nil), item.Embedding...)
	item.Metadata = maps.Clone(item.Metadata)
	return item
}

func cloneAssumption(link decision.AssumptionLink) decision.AssumptionLink {
	link.Metadata = maps.Clone(link.Metadata)
	return link
}

func cloneAlternative(alt decision.Alternative) decision.Alternative {
	alt.Metadata = maps.Clone(alt.Metadata)
	return alt
}

func cloneDecisionEvidence(ev decision.DecisionEvidence) decision.DecisionEvidence {
	ev.Metadata = maps.Clone(ev.Metadata)
	return ev
}

func cloneTransition(tr decision.Transition) decision.Transition {
	if tr.ActorUserID != nil {
		v := *tr.ActorUserID
		tr.ActorUserID = &v
	}
	return tr
}

func cloneCandidate(c decision.Candidate) decision.Candidate {
	c.Alternatives = append([]decision.Alternative(nil), c.Alternatives...)
	for i := range c.Alternatives {
		c.Alternatives[i] = cloneAlternative(c.Alternatives[i])
	}
	c.AssumptionHints = append([]string(nil), c.AssumptionHints...)
	c.EvidenceHints = append([]decision.EvidenceHint(nil), c.EvidenceHints...)
	c.Metadata = maps.Clone(c.Metadata)
	return c
}

func clampDecisionConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
