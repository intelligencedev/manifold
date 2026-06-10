package decision

import "context"

// Store persists decisions, assumptions, alternatives, evidence, transitions, and candidates.
type Store interface {
	Init(ctx context.Context) error
	UpsertDecision(ctx context.Context, d Decision) (Decision, error)
	GetDecision(ctx context.Context, tenantID int64, id string) (Decision, bool, error)
	SearchDecisions(ctx context.Context, q SearchQuery) ([]SearchResult, error)
	AddAssumption(ctx context.Context, a AssumptionLink) (AssumptionLink, error)
	ListAssumptions(ctx context.Context, q AssumptionQuery) ([]AssumptionLink, error)
	AddAlternative(ctx context.Context, alt Alternative) (Alternative, error)
	ListAlternatives(ctx context.Context, tenantID int64, decisionID string) ([]Alternative, error)
	AddEvidence(ctx context.Context, e DecisionEvidence) (DecisionEvidence, error)
	ListEvidence(ctx context.Context, q EvidenceQuery) ([]DecisionEvidence, error)
	RecordTransition(ctx context.Context, t Transition) (Transition, error)
	ListTransitions(ctx context.Context, q TransitionQuery) ([]Transition, error)
	RecordCandidate(ctx context.Context, c Candidate) (Candidate, error)
	GetCandidate(ctx context.Context, tenantID int64, id string) (Candidate, bool, error)
	ListCandidates(ctx context.Context, q CandidateQuery) ([]Candidate, error)
}

// NoopStore is a disabled decision store implementation.
type NoopStore struct{}

func (NoopStore) Init(context.Context) error { return nil }
func (NoopStore) UpsertDecision(_ context.Context, d Decision) (Decision, error) {
	return d, nil
}
func (NoopStore) GetDecision(context.Context, int64, string) (Decision, bool, error) {
	return Decision{}, false, nil
}
func (NoopStore) SearchDecisions(context.Context, SearchQuery) ([]SearchResult, error) {
	return nil, nil
}
func (NoopStore) AddAssumption(_ context.Context, a AssumptionLink) (AssumptionLink, error) {
	return a, nil
}
func (NoopStore) ListAssumptions(context.Context, AssumptionQuery) ([]AssumptionLink, error) {
	return nil, nil
}
func (NoopStore) AddAlternative(_ context.Context, alt Alternative) (Alternative, error) {
	return alt, nil
}
func (NoopStore) ListAlternatives(context.Context, int64, string) ([]Alternative, error) {
	return nil, nil
}
func (NoopStore) AddEvidence(_ context.Context, e DecisionEvidence) (DecisionEvidence, error) {
	return e, nil
}
func (NoopStore) ListEvidence(context.Context, EvidenceQuery) ([]DecisionEvidence, error) {
	return nil, nil
}
func (NoopStore) RecordTransition(_ context.Context, t Transition) (Transition, error) {
	return t, nil
}
func (NoopStore) ListTransitions(context.Context, TransitionQuery) ([]Transition, error) {
	return nil, nil
}
func (NoopStore) RecordCandidate(_ context.Context, c Candidate) (Candidate, error) {
	return c, nil
}
func (NoopStore) GetCandidate(context.Context, int64, string) (Candidate, bool, error) {
	return Candidate{}, false, nil
}
func (NoopStore) ListCandidates(context.Context, CandidateQuery) ([]Candidate, error) {
	return nil, nil
}
