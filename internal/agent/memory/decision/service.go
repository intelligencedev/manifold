package decision

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"manifold/internal/agent/memory/belief"

	"github.com/google/uuid"
)

var (
	// ErrStoreRequired is returned when a service method needs a backing store.
	ErrStoreRequired = errors.New("decision store is required")
	// ErrInvalidTransition is returned for illegal lifecycle transitions.
	ErrInvalidTransition = errors.New("invalid decision transition")
)

// Service enforces decision lifecycle invariants above the persistence layer.
type Service struct {
	Store  Store
	Belief belief.Store
	Config ServiceConfig
}

// ServiceConfig tunes decision candidate acceptance.
type ServiceConfig struct {
	AssumptionSimilarityFloor float64
	AutoActivateCandidates    bool
}

// CreateDecision normalizes and persists a decision.
func (s *Service) CreateDecision(ctx context.Context, d Decision) (Decision, error) {
	if s == nil || s.Store == nil {
		return Decision{}, ErrStoreRequired
	}
	d = normalizeDecision(d)
	existing, ok, err := findExistingDecision(ctx, s.Store, d)
	if err != nil {
		return Decision{}, err
	}
	if ok && strings.TrimSpace(d.ID) == "" {
		d.ID = existing.ID
		d.CreatedAt = existing.CreatedAt
	}
	created, err := s.Store.UpsertDecision(ctx, d)
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		_, err = s.Store.RecordTransition(ctx, Transition{
			TenantID:    created.TenantID,
			DecisionID:  created.ID,
			FromStatus:  "",
			ToStatus:    created.Status,
			Reason:      "decision recorded",
			TriggerKind: TriggerDistiller,
			TriggerID:   created.EpisodeID,
		})
	}
	return created, err
}

// TransitionDecision changes a decision status and appends an audit transition.
func (s *Service) TransitionDecision(ctx context.Context, tenantID int64, id string, to DecisionStatus, req TransitionRequest) (Decision, error) {
	if s == nil || s.Store == nil {
		return Decision{}, ErrStoreRequired
	}
	item, ok, err := s.Store.GetDecision(ctx, tenantID, strings.TrimSpace(id))
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		return Decision{}, fmt.Errorf("decision not found: %s", id)
	}
	to = NormalizeDecisionStatus(to)
	trigger := req.TriggerKind
	if trigger == "" {
		trigger = TriggerOperator
	}
	if !validTransition(item.Status, to, trigger) {
		return Decision{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, item.Status, to)
	}
	if item.Status == to && trigger != TriggerBeliefChange {
		return item, nil
	}
	from := item.Status
	item.Status = to
	if req.ReviewState != "" {
		item.ReviewState = NormalizeReviewState(req.ReviewState)
	}
	if to == DecisionStatusStale {
		item.StaleReason = strings.TrimSpace(req.Reason)
	}
	if to != DecisionStatusStale && req.ClearStaleReason {
		item.StaleReason = ""
	}
	if req.SupersededBy != "" {
		item.SupersededBy = strings.TrimSpace(req.SupersededBy)
	}
	item, err = s.Store.UpsertDecision(ctx, item)
	if err != nil {
		return Decision{}, err
	}
	_, err = s.Store.RecordTransition(ctx, Transition{
		TenantID:    item.TenantID,
		DecisionID:  item.ID,
		FromStatus:  from,
		ToStatus:    to,
		Reason:      strings.TrimSpace(req.Reason),
		TriggerKind: trigger,
		TriggerID:   strings.TrimSpace(req.TriggerID),
		ActorUserID: req.ActorUserID,
	})
	return item, err
}

// TransitionRequest carries transition audit metadata.
type TransitionRequest struct {
	Reason           string
	TriggerKind      TransitionTriggerKind
	TriggerID        string
	ActorUserID      *int64
	ReviewState      ReviewState
	SupersededBy     string
	ClearStaleReason bool
}

// MarkNeedsReview flags an active decision without changing status.
func (s *Service) MarkNeedsReview(ctx context.Context, tenantID int64, id, reason, triggerID string) (Decision, error) {
	if s == nil || s.Store == nil {
		return Decision{}, ErrStoreRequired
	}
	item, ok, err := s.Store.GetDecision(ctx, tenantID, strings.TrimSpace(id))
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		return Decision{}, fmt.Errorf("decision not found: %s", id)
	}
	if item.ReviewState == ReviewStateNeedsReview {
		return item, nil
	}
	return s.TransitionDecision(ctx, tenantID, id, item.Status, TransitionRequest{
		Reason:      reason,
		TriggerKind: TriggerBeliefChange,
		TriggerID:   triggerID,
		ReviewState: ReviewStateNeedsReview,
	})
}

// ReaffirmDecision moves a stale decision back to active with an operator audit row.
func (s *Service) ReaffirmDecision(ctx context.Context, tenantID int64, id, reason string, actorUserID *int64) (Decision, error) {
	return s.TransitionDecision(ctx, tenantID, id, DecisionStatusActive, TransitionRequest{
		Reason:           reason,
		TriggerKind:      TriggerOperator,
		ActorUserID:      actorUserID,
		ReviewState:      ReviewStateOperatorApproved,
		ClearStaleReason: true,
	})
}

// RevokeDecision revokes a proposed, active, or stale decision.
func (s *Service) RevokeDecision(ctx context.Context, tenantID int64, id, reason string, actorUserID *int64) (Decision, error) {
	return s.TransitionDecision(ctx, tenantID, id, DecisionStatusRevoked, TransitionRequest{
		Reason:      reason,
		TriggerKind: TriggerOperator,
		ActorUserID: actorUserID,
	})
}

// Supersede marks an old decision superseded by a new active or proposed decision.
func (s *Service) Supersede(ctx context.Context, tenantID int64, oldID string, replacement Decision, reason string, actorUserID *int64) (Decision, Decision, error) {
	if s == nil || s.Store == nil {
		return Decision{}, Decision{}, ErrStoreRequired
	}
	replacement.Status = NormalizeDecisionStatus(replacement.Status)
	if replacement.Status != DecisionStatusActive && replacement.Status != DecisionStatusProposed {
		return Decision{}, Decision{}, fmt.Errorf("replacement decision must be active or proposed")
	}
	replacement.TenantID = tenantID
	replacement, err := s.CreateDecision(ctx, replacement)
	if err != nil {
		return Decision{}, Decision{}, err
	}
	old, err := s.TransitionDecision(ctx, tenantID, oldID, DecisionStatusSuperseded, TransitionRequest{
		Reason:       reason,
		TriggerKind:  TriggerSupersession,
		TriggerID:    replacement.ID,
		ActorUserID:  actorUserID,
		SupersededBy: replacement.ID,
	})
	if err != nil {
		return Decision{}, Decision{}, err
	}
	return old, replacement, nil
}

// AcceptCandidate persists a candidate as an active decision and resolves assumptions.
func (s *Service) AcceptCandidate(ctx context.Context, tenantID int64, candidateID string, actorUserID *int64) (Decision, error) {
	if s == nil || s.Store == nil {
		return Decision{}, ErrStoreRequired
	}
	candidate, ok, err := s.Store.GetCandidate(ctx, tenantID, strings.TrimSpace(candidateID))
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		return Decision{}, fmt.Errorf("candidate not found: %s", candidateID)
	}
	status := DecisionStatusActive
	review := ReviewStateOperatorApproved
	if s.Config.AutoActivateCandidates && actorUserID == nil {
		review = ReviewStateAutoActive
	}
	created, err := s.CreateDecision(ctx, Decision{
		TenantID:      candidate.TenantID,
		ScopeID:       candidate.ScopeID,
		EpisodeID:     candidate.EpisodeID,
		Title:         candidate.Title,
		Statement:     candidate.Statement,
		StatementHash: candidate.StatementHash,
		Rationale:     candidate.Rationale,
		DecidedBy:     decidedBy(actorUserID),
		DecidedAt:     time.Now().UTC(),
		Status:        status,
		ReviewState:   review,
		Confidence:    candidate.Confidence,
		Metadata:      maps.Clone(candidate.Metadata),
	})
	if err != nil {
		return Decision{}, err
	}
	unresolved := make([]string, 0)
	for _, hint := range candidate.AssumptionHints {
		link, ok, err := s.resolveAssumptionHint(ctx, created, hint)
		if err != nil {
			return Decision{}, err
		}
		if !ok {
			unresolved = append(unresolved, strings.TrimSpace(hint))
			continue
		}
		if _, err := s.Store.AddAssumption(ctx, link); err != nil {
			return Decision{}, err
		}
	}
	for _, alt := range candidate.Alternatives {
		alt.TenantID = created.TenantID
		alt.DecisionID = created.ID
		if _, err := s.Store.AddAlternative(ctx, alt); err != nil {
			return Decision{}, err
		}
	}
	for _, hint := range candidate.EvidenceHints {
		if strings.TrimSpace(hint.SourceKind) == "" || strings.TrimSpace(hint.SourceID) == "" {
			continue
		}
		if _, err := s.Store.AddEvidence(ctx, DecisionEvidence{
			TenantID:   created.TenantID,
			DecisionID: created.ID,
			SourceKind: strings.TrimSpace(hint.SourceKind),
			SourceID:   strings.TrimSpace(hint.SourceID),
			Polarity:   NormalizeEvidencePolarity(hint.Polarity),
			Weight:     candidate.Confidence,
			Note:       strings.TrimSpace(hint.Note),
		}); err != nil {
			return Decision{}, err
		}
	}
	if len(unresolved) > 0 {
		if created.Metadata == nil {
			created.Metadata = map[string]any{}
		}
		created.Metadata["unresolvedAssumptions"] = unresolved
		created, err = s.Store.UpsertDecision(ctx, created)
		if err != nil {
			return Decision{}, err
		}
	}
	candidate.ValidationStatus = CandidateValidationAccepted
	candidate.AcceptedDecisionID = created.ID
	candidate.ReviewState = review
	_, err = s.Store.RecordCandidate(ctx, candidate)
	return created, err
}

func (s *Service) resolveAssumptionHint(ctx context.Context, d Decision, hint string) (AssumptionLink, bool, error) {
	hint = strings.TrimSpace(hint)
	if hint == "" || s.Belief == nil {
		return AssumptionLink{}, false, nil
	}
	floor := s.Config.AssumptionSimilarityFloor
	if floor <= 0 {
		floor = 0.80
	}
	results, err := s.Belief.SearchBeliefs(ctx, belief.SearchQuery{
		TenantID: d.TenantID,
		ScopeIDs: []string{d.ScopeID},
		Query:    hint,
		Statuses: []belief.BeliefStatus{belief.BeliefStatusActive},
		Limit:    1,
	})
	if err != nil {
		return AssumptionLink{}, false, err
	}
	if len(results) == 0 || results[0].Score < floor {
		return AssumptionLink{}, false, nil
	}
	item := results[0].Belief
	return AssumptionLink{
		TenantID:               d.TenantID,
		DecisionID:             d.ID,
		BeliefID:               item.ID,
		Criticality:            CriticalitySupporting,
		BeliefStatementAtLink:  item.Statement,
		BeliefConfidenceAtLink: item.Confidence,
		Note:                   hint,
	}, true, nil
}

func normalizeDecision(d Decision) Decision {
	now := time.Now().UTC()
	if strings.TrimSpace(d.ID) == "" {
		d.ID = uuid.NewString()
	}
	d.Statement = NormalizeStatement(d.Statement)
	if strings.TrimSpace(d.StatementHash) == "" {
		d.StatementHash = StatementHash(d.Statement)
	}
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" {
		d.Title = titleFromStatement(d.Statement)
	}
	d.Status = NormalizeDecisionStatus(d.Status)
	d.ReviewState = NormalizeReviewState(d.ReviewState)
	d.Confidence = clampConfidence(d.Confidence)
	if d.DecidedAt.IsZero() {
		d.DecidedAt = now
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	if d.Metadata == nil {
		d.Metadata = map[string]any{}
	}
	return d
}

func titleFromStatement(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return "Untitled decision"
	}
	if len([]rune(statement)) <= 96 {
		return strings.TrimSuffix(statement, ".")
	}
	runes := []rune(statement)
	return strings.TrimSpace(string(runes[:96])) + "..."
}

func findExistingDecision(ctx context.Context, store Store, d Decision) (Decision, bool, error) {
	if strings.TrimSpace(d.ScopeID) == "" || strings.TrimSpace(d.StatementHash) == "" {
		return Decision{}, false, nil
	}
	results, err := store.SearchDecisions(ctx, SearchQuery{
		TenantID: d.TenantID,
		ScopeIDs: []string{d.ScopeID},
		Query:    d.Statement,
		Statuses: []DecisionStatus{DecisionStatusProposed, DecisionStatusActive, DecisionStatusStale, DecisionStatusSuperseded, DecisionStatusRevoked},
		Limit:    20,
	})
	if err != nil {
		return Decision{}, false, err
	}
	for _, result := range results {
		if result.Decision.StatementHash == d.StatementHash {
			return result.Decision, true, nil
		}
	}
	return Decision{}, false, nil
}

func validTransition(from, to DecisionStatus, trigger TransitionTriggerKind) bool {
	if from == "" {
		return true
	}
	if from == to {
		return trigger == TriggerBeliefChange
	}
	switch NormalizeDecisionStatus(from) {
	case DecisionStatusProposed:
		return to == DecisionStatusActive || to == DecisionStatusRevoked
	case DecisionStatusActive:
		return to == DecisionStatusStale || to == DecisionStatusSuperseded || to == DecisionStatusRevoked
	case DecisionStatusStale:
		return to == DecisionStatusActive || to == DecisionStatusSuperseded || to == DecisionStatusRevoked
	default:
		return false
	}
}

func decidedBy(actorUserID *int64) string {
	if actorUserID == nil {
		return "agent:decision_distiller"
	}
	return fmt.Sprintf("human:%d", *actorUserID)
}
