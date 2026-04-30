package belief

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	RelationPromotedTo  = "PROMOTED_TO"
	RelationSupersedes  = "SUPERSEDES"
	RelationContradicts = "CONTRADICTS"
)

type Graph interface {
	UpsertNode(ctx context.Context, id string, labels []string, props map[string]any) error
	UpsertEdge(ctx context.Context, srcID, rel, dstID string, props map[string]any) error
	Neighbors(ctx context.Context, id, rel string) ([]string, error)
}

type LifecycleService struct {
	Store  Store
	Graph  Graph
	Policy PromotionPolicy
}

type PromotionPolicy struct {
	MinEvidenceFor       int
	MaxEvidenceAgainst   int
	ConfidenceThreshold  float64
	ScopeWideningDecay   float64
	StaleAfter           time.Duration
	StaleConfidenceDecay float64
}

type PromotionRequest struct {
	TenantID       int64
	BeliefID       string
	ToScope        Scope
	Reason         string
	ActorUserID    *int64
	ManualApproval bool
}

type LifecycleResult struct {
	Belief    Belief
	Promotion Promotion
}

func DefaultPromotionPolicy() PromotionPolicy {
	return PromotionPolicy{
		MinEvidenceFor:       2,
		MaxEvidenceAgainst:   0,
		ConfidenceThreshold:  0.80,
		ScopeWideningDecay:   0.85,
		StaleAfter:           30 * 24 * time.Hour,
		StaleConfidenceDecay: 0.90,
	}
}

func (s LifecycleService) Promote(ctx context.Context, request PromotionRequest) (LifecycleResult, error) {
	if s.Store == nil {
		return LifecycleResult{}, fmt.Errorf("belief store is required")
	}
	policy := s.policy()
	item, ok, err := s.Store.GetBelief(ctx, request.TenantID, strings.TrimSpace(request.BeliefID))
	if err != nil {
		return LifecycleResult{}, err
	}
	if !ok {
		return LifecycleResult{}, fmt.Errorf("belief not found: %s", request.BeliefID)
	}
	if request.ToScope.ID == "" {
		return LifecycleResult{}, fmt.Errorf("target scope is required")
	}
	fromScope, _, err := s.scopeForBelief(ctx, item)
	if err != nil {
		return LifecycleResult{}, err
	}
	if !canPromote(item, fromScope, request.ToScope, policy, request.ManualApproval) {
		return LifecycleResult{}, fmt.Errorf("belief does not satisfy promotion policy")
	}
	confidenceBefore := item.Confidence
	promoted := item
	promoted.ID = ""
	promoted.ScopeID = request.ToScope.ID
	promoted.Confidence = clampConfidence(item.Confidence * policy.ScopeWideningDecay)
	promoted.Status = BeliefStatusActive
	promoted.Metadata = cloneMap(item.Metadata)
	promoted.Metadata["promotedFromBeliefId"] = item.ID
	promoted.Metadata["promotedFromScopeId"] = item.ScopeID
	promoted.Metadata["promotionReason"] = strings.TrimSpace(request.Reason)
	promoted.CreatedAt = time.Time{}
	promoted.UpdatedAt = time.Time{}
	promoted, err = s.Store.UpsertBelief(ctx, promoted)
	if err != nil {
		return LifecycleResult{}, err
	}
	promotion, err := s.Store.RecordPromotion(ctx, Promotion{
		TenantID:         item.TenantID,
		BeliefID:         promoted.ID,
		FromScope:        item.ScopeID,
		ToScope:          request.ToScope.ID,
		Reason:           strings.TrimSpace(request.Reason),
		ConfidenceBefore: confidenceBefore,
		ConfidenceAfter:  promoted.Confidence,
		ActorUserID:      request.ActorUserID,
		Metadata: map[string]any{
			"sourceBeliefId": item.ID,
			"targetKind":     request.ToScope.Kind,
		},
	})
	if err != nil {
		return LifecycleResult{}, err
	}
	_ = s.mirrorPromotion(ctx, item, promoted, promotion)
	log.Ctx(ctx).Info().Str("belief_id", item.ID).Str("promoted_belief_id", promoted.ID).Str("from_scope", item.ScopeID).Str("to_scope", request.ToScope.ID).Msg("belief_lifecycle_promoted")
	return LifecycleResult{Belief: promoted, Promotion: promotion}, nil
}

func (s LifecycleService) DecayStale(ctx context.Context, query SearchQuery, now time.Time) ([]Belief, error) {
	if s.Store == nil {
		return nil, fmt.Errorf("belief store is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	policy := s.policy()
	query.Statuses = []BeliefStatus{BeliefStatusActive}
	if query.Limit <= 0 {
		query.Limit = 100
	}
	results, err := s.Store.SearchBeliefs(ctx, query)
	if err != nil {
		return nil, err
	}
	changed := make([]Belief, 0)
	for _, result := range results {
		item := result.Belief
		if item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
			item.Status = BeliefStatusRetracted
			item.Metadata = cloneMap(item.Metadata)
			item.Metadata["expiredAt"] = now.Format(time.RFC3339)
		} else if isStale(item, policy, now) {
			item.Confidence = clampConfidence(item.Confidence * policy.StaleConfidenceDecay)
			item.Metadata = cloneMap(item.Metadata)
			item.Metadata["decayedAt"] = now.Format(time.RFC3339)
		} else {
			continue
		}
		updated, err := s.Store.UpsertBelief(ctx, item)
		if err != nil {
			return changed, err
		}
		log.Ctx(ctx).Info().Str("belief_id", updated.ID).Str("status", string(updated.Status)).Float64("confidence", updated.Confidence).Msg("belief_lifecycle_stale_processed")
		changed = append(changed, updated)
	}
	return changed, nil
}

func (s LifecycleService) Retract(ctx context.Context, tenantID int64, beliefID, reason string, actorUserID *int64) (Belief, error) {
	if s.Store == nil {
		return Belief{}, fmt.Errorf("belief store is required")
	}
	item, ok, err := s.Store.GetBelief(ctx, tenantID, strings.TrimSpace(beliefID))
	if err != nil {
		return Belief{}, err
	}
	if !ok {
		return Belief{}, fmt.Errorf("belief not found: %s", beliefID)
	}
	item.Status = BeliefStatusRetracted
	item.Metadata = cloneMap(item.Metadata)
	item.Metadata["retractionReason"] = strings.TrimSpace(reason)
	if actorUserID != nil {
		item.Metadata["retractedBy"] = *actorUserID
	}
	item.Metadata["retractedAt"] = time.Now().UTC().Format(time.RFC3339)
	updated, err := s.Store.UpsertBelief(ctx, item)
	if err == nil {
		log.Ctx(ctx).Info().Str("belief_id", updated.ID).Msg("belief_lifecycle_retracted")
	}
	return updated, err
}

func (s LifecycleService) Supersede(ctx context.Context, tenantID int64, oldBeliefID string, replacement Belief, reason string) (Belief, Belief, error) {
	if s.Store == nil {
		return Belief{}, Belief{}, fmt.Errorf("belief store is required")
	}
	oldBelief, ok, err := s.Store.GetBelief(ctx, tenantID, strings.TrimSpace(oldBeliefID))
	if err != nil {
		return Belief{}, Belief{}, err
	}
	if !ok {
		return Belief{}, Belief{}, fmt.Errorf("belief not found: %s", oldBeliefID)
	}
	replacement.TenantID = tenantID
	if replacement.ScopeID == "" {
		replacement.ScopeID = oldBelief.ScopeID
	}
	if replacement.StatementHash == "" {
		replacement.StatementHash = StatementHash(replacement.Statement)
	}
	if replacement.Status == "" {
		replacement.Status = BeliefStatusActive
	}
	replacement.Metadata = cloneMap(replacement.Metadata)
	replacement.Metadata["supersedesBeliefId"] = oldBelief.ID
	replacement.Metadata["supersessionReason"] = strings.TrimSpace(reason)
	replacement, err = s.Store.UpsertBelief(ctx, replacement)
	if err != nil {
		return Belief{}, Belief{}, err
	}
	oldBelief.Status = BeliefStatusSuperseded
	oldBelief.Metadata = cloneMap(oldBelief.Metadata)
	oldBelief.Metadata["supersededByBeliefId"] = replacement.ID
	oldBelief.Metadata["supersessionReason"] = strings.TrimSpace(reason)
	oldBelief, err = s.Store.UpsertBelief(ctx, oldBelief)
	if err != nil {
		return Belief{}, Belief{}, err
	}
	_ = s.mirrorRelation(ctx, replacement.ID, RelationSupersedes, oldBelief.ID, map[string]any{"reason": strings.TrimSpace(reason)})
	log.Ctx(ctx).Info().Str("belief_id", oldBelief.ID).Str("replacement_belief_id", replacement.ID).Msg("belief_lifecycle_superseded")
	return oldBelief, replacement, nil
}

func (s LifecycleService) MirrorContradiction(ctx context.Context, sourceID, targetID, evidenceID string) error {
	return s.mirrorRelation(ctx, sourceID, RelationContradicts, targetID, map[string]any{"evidenceId": strings.TrimSpace(evidenceID)})
}

func (s LifecycleService) policy() PromotionPolicy {
	policy := s.Policy
	defaults := DefaultPromotionPolicy()
	if policy.MinEvidenceFor <= 0 {
		policy.MinEvidenceFor = defaults.MinEvidenceFor
	}
	if policy.ConfidenceThreshold <= 0 {
		policy.ConfidenceThreshold = defaults.ConfidenceThreshold
	}
	if policy.ScopeWideningDecay <= 0 || policy.ScopeWideningDecay > 1 {
		policy.ScopeWideningDecay = defaults.ScopeWideningDecay
	}
	if policy.StaleAfter <= 0 {
		policy.StaleAfter = defaults.StaleAfter
	}
	if policy.StaleConfidenceDecay <= 0 || policy.StaleConfidenceDecay > 1 {
		policy.StaleConfidenceDecay = defaults.StaleConfidenceDecay
	}
	return policy
}

func (s LifecycleService) scopeForBelief(ctx context.Context, item Belief) (Scope, bool, error) {
	return s.Store.GetScopeByID(ctx, item.TenantID, item.ScopeID)
}

func canPromote(item Belief, fromScope, toScope Scope, policy PromotionPolicy, manual bool) bool {
	if item.Status != BeliefStatusActive || item.Confidence < policy.ConfidenceThreshold || item.EvidenceFor < policy.MinEvidenceFor || item.EvidenceAgainst > policy.MaxEvidenceAgainst {
		return false
	}
	if toScope.Kind == ScopeKindOrg && !manual {
		return false
	}
	if fromScope.Kind == ScopeKindObjective && toScope.Kind == ScopeKindProject {
		return true
	}
	if fromScope.Kind == ScopeKindProject && toScope.Kind == ScopeKindOrg {
		return manual
	}
	if fromScope.Kind == "" {
		return toScope.Kind == ScopeKindProject || (toScope.Kind == ScopeKindOrg && manual)
	}
	return false
}

func isStale(item Belief, policy PromotionPolicy, now time.Time) bool {
	observed := item.UpdatedAt
	if item.LastObserved != nil {
		observed = *item.LastObserved
	}
	if observed.IsZero() {
		return false
	}
	return now.Sub(observed) >= policy.StaleAfter
}

func (s LifecycleService) mirrorPromotion(ctx context.Context, source, target Belief, promotion Promotion) error {
	if s.Graph == nil {
		return nil
	}
	if err := s.Graph.UpsertNode(ctx, source.ID, []string{"Belief"}, graphBeliefProps(source)); err != nil {
		return err
	}
	if err := s.Graph.UpsertNode(ctx, target.ID, []string{"Belief"}, graphBeliefProps(target)); err != nil {
		return err
	}
	return s.mirrorRelation(ctx, source.ID, RelationPromotedTo, target.ID, map[string]any{"promotionId": promotion.ID})
}

func (s LifecycleService) mirrorRelation(ctx context.Context, sourceID, relation, targetID string, props map[string]any) error {
	if s.Graph == nil || strings.TrimSpace(sourceID) == "" || strings.TrimSpace(targetID) == "" {
		return nil
	}
	return s.Graph.UpsertEdge(ctx, sourceID, relation, targetID, props)
}

func graphBeliefProps(item Belief) map[string]any {
	return map[string]any{
		"tenantId":   item.TenantID,
		"scopeId":    item.ScopeID,
		"statement":  item.Statement,
		"confidence": item.Confidence,
		"status":     item.Status,
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
