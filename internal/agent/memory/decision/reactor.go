package decision

import (
	"context"
	"fmt"
	"strings"

	"manifold/internal/agent/memory/belief"
)

// Reactor reacts to belief changes by flagging dependent decisions.
type Reactor struct {
	Decisions *Service
	Beliefs   belief.Store
	Config    ReactorConfig
}

// ReactorConfig tunes confidence-based decision reactivity.
type ReactorConfig struct {
	ConfidenceFloor     float64
	ConfidenceDropDelta float64
}

// OnBeliefChanged handles one belief change event.
func (r *Reactor) OnBeliefChanged(ctx context.Context, ev belief.ChangeEvent) {
	if r == nil || r.Decisions == nil || r.Decisions.Store == nil {
		return
	}
	links, err := r.Decisions.Store.ListAssumptions(ctx, AssumptionQuery{TenantID: ev.TenantID, BeliefID: ev.BeliefID})
	if err != nil {
		return
	}
	for _, link := range links {
		r.reactToLink(ctx, ev, link)
	}
}

func (r *Reactor) reactToLink(ctx context.Context, ev belief.ChangeEvent, link AssumptionLink) {
	criticality := NormalizeAssumptionCriticality(link.Criticality)
	switch ev.Kind {
	case belief.ChangeStatus:
		if ev.After.Status == belief.BeliefStatusRetracted || ev.After.Status == belief.BeliefStatusSuperseded {
			switch criticality {
			case CriticalityLoadBearing:
				r.markStale(ctx, ev, link, fmt.Sprintf("Load-bearing belief changed from %s to %s: %s", ev.Before.Status, ev.After.Status, link.BeliefStatementAtLink))
			case CriticalitySupporting:
				r.needsReview(ctx, ev, link, fmt.Sprintf("Supporting belief changed from %s to %s: %s", ev.Before.Status, ev.After.Status, link.BeliefStatementAtLink))
			default:
				r.auditOnly(ctx, ev, link, fmt.Sprintf("Contextual belief changed from %s to %s", ev.Before.Status, ev.After.Status))
			}
		}
	case belief.ChangeExpiry:
		switch criticality {
		case CriticalityLoadBearing:
			r.markStale(ctx, ev, link, "Load-bearing belief expired: "+link.BeliefStatementAtLink)
		case CriticalitySupporting:
			r.needsReview(ctx, ev, link, "Supporting belief expired: "+link.BeliefStatementAtLink)
		default:
			r.auditOnly(ctx, ev, link, "Contextual belief expired")
		}
	case belief.ChangeConfidence:
		if r.confidenceChangedEnough(ev.After.Confidence, link.BeliefConfidenceAtLink) {
			switch criticality {
			case CriticalityLoadBearing:
				r.needsReview(ctx, ev, link, fmt.Sprintf("Load-bearing belief confidence changed from %.2f to %.2f: %s", link.BeliefConfidenceAtLink, ev.After.Confidence, link.BeliefStatementAtLink))
			case CriticalitySupporting:
				r.auditOnly(ctx, ev, link, fmt.Sprintf("Supporting belief confidence changed from %.2f to %.2f", link.BeliefConfidenceAtLink, ev.After.Confidence))
			}
		}
	}
}

func (r *Reactor) confidenceChangedEnough(current, snapshot float64) bool {
	floor := r.Config.ConfidenceFloor
	if floor <= 0 {
		floor = 0.35
	}
	delta := r.Config.ConfidenceDropDelta
	if delta <= 0 {
		delta = 0.30
	}
	return current < floor || snapshot-current >= delta
}

func (r *Reactor) markStale(ctx context.Context, ev belief.ChangeEvent, link AssumptionLink, reason string) {
	item, ok, err := r.Decisions.Store.GetDecision(ctx, ev.TenantID, link.DecisionID)
	if err != nil || !ok || item.Status == DecisionStatusStale || item.Status == DecisionStatusSuperseded || item.Status == DecisionStatusRevoked {
		return
	}
	_, _ = r.Decisions.TransitionDecision(ctx, ev.TenantID, link.DecisionID, DecisionStatusStale, TransitionRequest{
		Reason:      strings.TrimSpace(reason),
		TriggerKind: TriggerBeliefChange,
		TriggerID:   ev.BeliefID,
		ReviewState: ReviewStateNeedsReview,
	})
}

func (r *Reactor) needsReview(ctx context.Context, ev belief.ChangeEvent, link AssumptionLink, reason string) {
	_, _ = r.Decisions.MarkNeedsReview(ctx, ev.TenantID, link.DecisionID, strings.TrimSpace(reason), ev.BeliefID)
}

func (r *Reactor) auditOnly(ctx context.Context, ev belief.ChangeEvent, link AssumptionLink, reason string) {
	item, ok, err := r.Decisions.Store.GetDecision(ctx, ev.TenantID, link.DecisionID)
	if err != nil || !ok {
		return
	}
	_, _ = r.Decisions.Store.RecordTransition(ctx, Transition{
		TenantID:    ev.TenantID,
		DecisionID:  link.DecisionID,
		FromStatus:  item.Status,
		ToStatus:    item.Status,
		Reason:      strings.TrimSpace(reason),
		TriggerKind: TriggerBeliefChange,
		TriggerID:   ev.BeliefID,
	})
}
