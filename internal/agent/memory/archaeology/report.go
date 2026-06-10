package archaeology

import (
	"time"

	"manifold/internal/agent/memory/artifact"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
)

// ReconstructOptions tunes a decision archaeology query.
type ReconstructOptions struct {
	ScopeIDs     []string
	AsOf         *time.Time
	IncludeStale bool
	MaxDecisions int
}

// Report is the top-level reconstruction result.
type Report struct {
	Decisions []DecisionDossier `json:"decisions"`
}

// DecisionDossier contains the recovered context around one decision.
type DecisionDossier struct {
	Decision     decision.Decision      `json:"decision"`
	Transitions  []decision.Transition  `json:"transitions,omitempty"`
	Assumptions  []AssumptionStatus     `json:"assumptions,omitempty"`
	Alternatives []decision.Alternative `json:"alternatives,omitempty"`
	Evidence     []EvidenceResolved     `json:"evidence,omitempty"`
	Verdict      string                 `json:"verdict"`
	Narrative    string                 `json:"narrative,omitempty"`
}

// AssumptionStatus compares the belief snapshot at decision time with current state.
type AssumptionStatus struct {
	Link       decision.AssumptionLink `json:"link"`
	BeliefNow  *belief.Belief          `json:"beliefNow,omitempty"`
	StillHolds bool                    `json:"stillHolds"`
}

// EvidenceResolved resolves known decision evidence source types.
type EvidenceResolved struct {
	Evidence decision.DecisionEvidence `json:"evidence"`
	Artifact *artifact.Artifact        `json:"artifact,omitempty"`
	Belief   *belief.Belief            `json:"belief,omitempty"`
	Episode  *belief.Episode           `json:"episode,omitempty"`
}
