package archaeology

import (
	"context"
	"strings"
	"time"

	"manifold/internal/agent/memory/artifact"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
)

// Service reconstructs why a decision exists and whether its assumptions hold.
type Service struct {
	Decisions *decision.Service
	Beliefs   belief.Store
	Artifacts artifact.Store
	Config    ServiceConfig
}

// ServiceConfig tunes reconstruction verdicts.
type ServiceConfig struct {
	ConfidenceFloor float64
}

// Reconstruct answers why a decision exists.
func (s *Service) Reconstruct(ctx context.Context, tenantID int64, query string, opts ReconstructOptions) (Report, error) {
	if s == nil || s.Decisions == nil || s.Decisions.Store == nil {
		return Report{}, decision.ErrStoreRequired
	}
	limit := opts.MaxDecisions
	if limit <= 0 {
		limit = 5
	}
	statuses := []decision.DecisionStatus{
		decision.DecisionStatusProposed,
		decision.DecisionStatusActive,
		decision.DecisionStatusStale,
		decision.DecisionStatusSuperseded,
		decision.DecisionStatusRevoked,
	}
	if !opts.IncludeStale {
		statuses = []decision.DecisionStatus{decision.DecisionStatusActive, decision.DecisionStatusProposed}
	}
	results, err := s.Decisions.Store.SearchDecisions(ctx, decision.SearchQuery{
		TenantID: tenantID,
		ScopeIDs: opts.ScopeIDs,
		Query:    query,
		Statuses: statuses,
		Limit:    limit,
	})
	if err != nil {
		return Report{}, err
	}
	report := Report{Decisions: make([]DecisionDossier, 0, len(results))}
	for _, result := range results {
		dossier, err := s.loadDossier(ctx, tenantID, result.Decision.ID, opts)
		if err != nil {
			return Report{}, err
		}
		report.Decisions = append(report.Decisions, dossier)
	}
	return report, nil
}

func (s *Service) loadDossier(ctx context.Context, tenantID int64, decisionID string, opts ReconstructOptions) (DecisionDossier, error) {
	lineage, ok, err := s.Decisions.LoadLineage(ctx, tenantID, decisionID)
	if err != nil || !ok {
		return DecisionDossier{}, err
	}
	item := lineage.Decision
	if opts.AsOf != nil {
		item.Status = statusAsOf(item.Status, lineage.Transitions, *opts.AsOf)
	}
	assumptions := make([]AssumptionStatus, 0, len(lineage.Assumptions))
	for _, link := range lineage.Assumptions {
		status := AssumptionStatus{Link: link, StillHolds: true}
		if s.Beliefs != nil && strings.TrimSpace(link.BeliefID) != "" {
			if b, ok, err := s.Beliefs.GetBelief(ctx, tenantID, link.BeliefID); err == nil && ok {
				status.BeliefNow = &b
				status.StillHolds = b.Status == belief.BeliefStatusActive && b.Confidence >= s.confidenceFloor()
			} else {
				status.StillHolds = false
			}
		}
		assumptions = append(assumptions, status)
	}
	evidence := make([]EvidenceResolved, 0, len(lineage.Evidence))
	for _, ev := range lineage.Evidence {
		resolved := EvidenceResolved{Evidence: ev}
		switch ev.SourceKind {
		case string(belief.SourceKindArtifact):
			if s.Artifacts != nil {
				if item, ok, err := s.Artifacts.GetArtifact(ctx, tenantID, ev.SourceID); err == nil && ok {
					resolved.Artifact = &item
				}
			}
		case string(belief.SourceKindBelief):
			if s.Beliefs != nil {
				if item, ok, err := s.Beliefs.GetBelief(ctx, tenantID, ev.SourceID); err == nil && ok {
					resolved.Belief = &item
				}
			}
		case string(belief.SourceKindEpisode):
			if s.Beliefs != nil {
				if item, ok, err := s.Beliefs.GetEpisode(ctx, tenantID, ev.SourceID); err == nil && ok {
					resolved.Episode = &item
				}
			}
		}
		evidence = append(evidence, resolved)
	}
	dossier := DecisionDossier{
		Decision:     item,
		Transitions:  lineage.Transitions,
		Assumptions:  assumptions,
		Alternatives: lineage.Alternatives,
		Evidence:     evidence,
	}
	dossier.Verdict = verdict(item, assumptions)
	return dossier, nil
}

func statusAsOf(current decision.DecisionStatus, transitions []decision.Transition, asOfTime time.Time) decision.DecisionStatus {
	status := current
	for _, tr := range transitions {
		if tr.CreatedAt.After(asOfTime) {
			break
		}
		status = tr.ToStatus
	}
	if status == "" {
		return decision.DecisionStatusActive
	}
	return status
}

func verdict(item decision.Decision, assumptions []AssumptionStatus) string {
	switch item.Status {
	case decision.DecisionStatusSuperseded:
		return "superseded"
	case decision.DecisionStatusRevoked:
		return "revoked"
	case decision.DecisionStatusStale:
		return "stale"
	}
	if item.ReviewState == decision.ReviewStateNeedsReview {
		return "needs_review"
	}
	for _, assumption := range assumptions {
		if decision.NormalizeAssumptionCriticality(assumption.Link.Criticality) == decision.CriticalityLoadBearing && !assumption.StillHolds {
			return "stale"
		}
	}
	return "holds"
}

func (s *Service) confidenceFloor() float64 {
	if s == nil || s.Config.ConfidenceFloor <= 0 {
		return 0.35
	}
	return s.Config.ConfidenceFloor
}
