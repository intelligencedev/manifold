package decision

import "context"

// Lineage contains the persisted context around a decision.
type Lineage struct {
	Decision     Decision           `json:"decision"`
	Assumptions  []AssumptionLink   `json:"assumptions,omitempty"`
	Alternatives []Alternative      `json:"alternatives,omitempty"`
	Evidence     []DecisionEvidence `json:"evidence,omitempty"`
	Transitions  []Transition       `json:"transitions,omitempty"`
}

// LoadLineage loads the decision record and its immediate archaeology rows.
func (s *Service) LoadLineage(ctx context.Context, tenantID int64, id string) (Lineage, bool, error) {
	if s == nil || s.Store == nil {
		return Lineage{}, false, ErrStoreRequired
	}
	item, ok, err := s.Store.GetDecision(ctx, tenantID, id)
	if err != nil || !ok {
		return Lineage{}, ok, err
	}
	assumptions, err := s.Store.ListAssumptions(ctx, AssumptionQuery{TenantID: tenantID, DecisionID: id})
	if err != nil {
		return Lineage{}, false, err
	}
	alternatives, err := s.Store.ListAlternatives(ctx, tenantID, id)
	if err != nil {
		return Lineage{}, false, err
	}
	evidence, err := s.Store.ListEvidence(ctx, EvidenceQuery{TenantID: tenantID, DecisionID: id})
	if err != nil {
		return Lineage{}, false, err
	}
	transitions, err := s.Store.ListTransitions(ctx, TransitionQuery{TenantID: tenantID, DecisionID: id})
	if err != nil {
		return Lineage{}, false, err
	}
	return Lineage{
		Decision:     item,
		Assumptions:  assumptions,
		Alternatives: alternatives,
		Evidence:     evidence,
		Transitions:  transitions,
	}, true, nil
}
