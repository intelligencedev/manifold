package decision

import (
	"strings"
	"time"
)

// DecisionStatus describes where a recorded decision sits in its lifecycle.
type DecisionStatus string

const (
	// DecisionStatusProposed marks a drafted decision that is not yet in force.
	DecisionStatusProposed DecisionStatus = "proposed"
	// DecisionStatusActive marks a confirmed decision that should guide future work.
	DecisionStatusActive DecisionStatus = "active"
	// DecisionStatusStale marks a decision whose load-bearing assumptions changed.
	DecisionStatusStale DecisionStatus = "stale"
	// DecisionStatusSuperseded marks a decision replaced by another decision.
	DecisionStatusSuperseded DecisionStatus = "superseded"
	// DecisionStatusRevoked marks a decision explicitly reversed.
	DecisionStatusRevoked DecisionStatus = "revoked"
)

// ReviewState mirrors belief.ReviewState values for operator workflows.
type ReviewState string

const (
	// ReviewStateAutoActive marks records accepted by an automated policy.
	ReviewStateAutoActive ReviewState = "auto_active"
	// ReviewStateNeedsReview marks records requiring operator review.
	ReviewStateNeedsReview ReviewState = "needs_review"
	// ReviewStateOperatorApproved marks records approved by an operator.
	ReviewStateOperatorApproved ReviewState = "operator_approved"
	// ReviewStateOperatorRejected marks records rejected by an operator.
	ReviewStateOperatorRejected ReviewState = "operator_rejected"
)

// CandidateValidationStatus describes whether a distiller candidate was accepted.
type CandidateValidationStatus string

const (
	// CandidateValidationAccepted marks a candidate accepted into a decision.
	CandidateValidationAccepted CandidateValidationStatus = "accepted"
	// CandidateValidationQueued marks a candidate awaiting operator action.
	CandidateValidationQueued CandidateValidationStatus = "queued"
	// CandidateValidationRejected marks a candidate rejected during validation.
	CandidateValidationRejected CandidateValidationStatus = "rejected"
)

// EvidencePolarity describes whether evidence supports or contradicts a decision.
type EvidencePolarity string

const (
	// EvidencePolarityFor marks supporting evidence.
	EvidencePolarityFor EvidencePolarity = "for"
	// EvidencePolarityAgainst marks contradicting evidence.
	EvidencePolarityAgainst EvidencePolarity = "against"
)

// AssumptionCriticality controls how belief changes affect a decision.
type AssumptionCriticality string

const (
	// CriticalityLoadBearing means invalidating the belief stales the decision.
	CriticalityLoadBearing AssumptionCriticality = "load_bearing"
	// CriticalitySupporting means invalidating the belief flags the decision for review.
	CriticalitySupporting AssumptionCriticality = "supporting"
	// CriticalityContextual means the belief is informational only.
	CriticalityContextual AssumptionCriticality = "contextual"
)

// TransitionTriggerKind identifies what caused a decision lifecycle audit row.
type TransitionTriggerKind string

const (
	// TriggerBeliefChange marks a transition caused by a belief status or confidence change.
	TriggerBeliefChange TransitionTriggerKind = "belief_change"
	// TriggerOperator marks an operator-initiated transition.
	TriggerOperator TransitionTriggerKind = "operator"
	// TriggerSupersession marks a transition caused by another decision replacing this one.
	TriggerSupersession TransitionTriggerKind = "supersession"
	// TriggerDistiller marks a transition caused by candidate distillation.
	TriggerDistiller TransitionTriggerKind = "distiller"
)

// Decision is the first-class "we chose X" record.
type Decision struct {
	ID            string         `json:"id"`
	TenantID      int64          `json:"tenantId"`
	ScopeID       string         `json:"scopeId"`
	EpisodeID     string         `json:"episodeId,omitempty"`
	Title         string         `json:"title"`
	Statement     string         `json:"statement"`
	StatementHash string         `json:"statementHash"`
	Rationale     string         `json:"rationale"`
	DecidedBy     string         `json:"decidedBy"`
	DecidedAt     time.Time      `json:"decidedAt"`
	Status        DecisionStatus `json:"status"`
	ReviewState   ReviewState    `json:"reviewState"`
	Confidence    float64        `json:"confidence"`
	SupersededBy  string         `json:"supersededBy,omitempty"`
	StaleReason   string         `json:"staleReason,omitempty"`
	Embedding     []float32      `json:"-"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

// AssumptionLink ties a decision to a belief it depends on.
type AssumptionLink struct {
	ID                     string                `json:"id"`
	TenantID               int64                 `json:"tenantId"`
	DecisionID             string                `json:"decisionId"`
	BeliefID               string                `json:"beliefId"`
	Criticality            AssumptionCriticality `json:"criticality"`
	BeliefStatementAtLink  string                `json:"beliefStatementAtLink"`
	BeliefConfidenceAtLink float64               `json:"beliefConfidenceAtLink"`
	Note                   string                `json:"note,omitempty"`
	Metadata               map[string]any        `json:"metadata,omitempty"`
	CreatedAt              time.Time             `json:"createdAt"`
}

// Alternative records a path not taken and why.
type Alternative struct {
	ID              string         `json:"id"`
	TenantID        int64          `json:"tenantId"`
	DecisionID      string         `json:"decisionId"`
	Statement       string         `json:"statement"`
	RejectionReason string         `json:"rejectionReason"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
}

// DecisionEvidence links a decision to artifacts or other sources.
type DecisionEvidence struct {
	ID         string           `json:"id"`
	TenantID   int64            `json:"tenantId"`
	DecisionID string           `json:"decisionId"`
	SourceKind string           `json:"sourceKind"`
	SourceID   string           `json:"sourceId"`
	Polarity   EvidencePolarity `json:"polarity"`
	Weight     float64          `json:"weight"`
	Note       string           `json:"note"`
	Metadata   map[string]any   `json:"metadata,omitempty"`
	CreatedAt  time.Time        `json:"createdAt"`
}

// Transition is an append-only audit record for decision lifecycle changes.
type Transition struct {
	ID          string                `json:"id"`
	TenantID    int64                 `json:"tenantId"`
	DecisionID  string                `json:"decisionId"`
	FromStatus  DecisionStatus        `json:"fromStatus"`
	ToStatus    DecisionStatus        `json:"toStatus"`
	Reason      string                `json:"reason"`
	TriggerKind TransitionTriggerKind `json:"triggerKind"`
	TriggerID   string                `json:"triggerId"`
	ActorUserID *int64                `json:"actorUserId,omitempty"`
	CreatedAt   time.Time             `json:"createdAt"`
}

// Candidate is a distiller-proposed decision awaiting validation or acceptance.
type Candidate struct {
	ID                 string                    `json:"id"`
	TenantID           int64                     `json:"tenantId"`
	EpisodeID          string                    `json:"episodeId"`
	ScopeID            string                    `json:"scopeId"`
	Title              string                    `json:"title"`
	Statement          string                    `json:"statement"`
	StatementHash      string                    `json:"statementHash"`
	Rationale          string                    `json:"rationale"`
	Alternatives       []Alternative             `json:"alternatives,omitempty"`
	AssumptionHints    []string                  `json:"assumptionHints,omitempty"`
	EvidenceHints      []EvidenceHint            `json:"evidenceHints,omitempty"`
	Confidence         float64                   `json:"confidence"`
	ReviewState        ReviewState               `json:"reviewState"`
	ValidationStatus   CandidateValidationStatus `json:"validationStatus"`
	RejectionReason    string                    `json:"rejectionReason,omitempty"`
	AcceptedDecisionID string                    `json:"acceptedDecisionId,omitempty"`
	Model              string                    `json:"model,omitempty"`
	RawPayload         string                    `json:"rawPayload,omitempty"`
	Metadata           map[string]any            `json:"metadata,omitempty"`
	CreatedAt          time.Time                 `json:"createdAt"`
	UpdatedAt          time.Time                 `json:"updatedAt"`
}

// EvidenceHint is a distiller-proposed evidence link.
type EvidenceHint struct {
	SourceKind string           `json:"sourceKind"`
	SourceID   string           `json:"sourceId"`
	Polarity   EvidencePolarity `json:"polarity"`
	Note       string           `json:"note,omitempty"`
}

// SearchQuery constrains decision retrieval by tenant, scope, and status.
type SearchQuery struct {
	TenantID int64    `json:"tenantId"`
	ScopeIDs []string `json:"scopeIds,omitempty"`
	// ScopePrefixes optionally match decisions whose scope ID starts with any
	// prefix. This lets hierarchical path-style scope IDs (for example
	// "project/manifold/memory/archaeology") participate in scope walks.
	ScopePrefixes []string         `json:"scopePrefixes,omitempty"`
	Query         string           `json:"query,omitempty"`
	Statuses      []DecisionStatus `json:"statuses,omitempty"`
	Limit         int              `json:"limit,omitempty"`
}

// MatchesScope reports whether a decision scope ID passes the query's scope
// filters. A query without any non-empty scope filter matches every scope.
func (q SearchQuery) MatchesScope(scopeID string) bool {
	scopeID = strings.TrimSpace(scopeID)
	hasFilter := false
	for _, id := range q.ScopeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		hasFilter = true
		if id == scopeID {
			return true
		}
	}
	for _, prefix := range q.ScopePrefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		hasFilter = true
		if strings.HasPrefix(scopeID, prefix) {
			return true
		}
	}
	return !hasFilter
}

// AssumptionQuery filters decision assumption links.
type AssumptionQuery struct {
	TenantID   int64  `json:"tenantId"`
	DecisionID string `json:"decisionId,omitempty"`
	BeliefID   string `json:"beliefId,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// EvidenceQuery filters decision evidence links.
type EvidenceQuery struct {
	TenantID   int64            `json:"tenantId"`
	DecisionID string           `json:"decisionId,omitempty"`
	SourceKind string           `json:"sourceKind,omitempty"`
	SourceID   string           `json:"sourceId,omitempty"`
	Polarity   EvidencePolarity `json:"polarity,omitempty"`
	Limit      int              `json:"limit,omitempty"`
}

// TransitionQuery filters decision transition audit rows.
type TransitionQuery struct {
	TenantID   int64  `json:"tenantId"`
	DecisionID string `json:"decisionId,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// CandidateQuery filters decision candidates.
type CandidateQuery struct {
	TenantID         int64                     `json:"tenantId"`
	EpisodeID        string                    `json:"episodeId,omitempty"`
	ReviewState      ReviewState               `json:"reviewState,omitempty"`
	ValidationStatus CandidateValidationStatus `json:"validationStatus,omitempty"`
	Limit            int                       `json:"limit,omitempty"`
}

// SearchResult is a scored decision retrieval result.
type SearchResult struct {
	Decision Decision `json:"decision"`
	Score    float64  `json:"score"`
	Reason   string   `json:"reason,omitempty"`
}
