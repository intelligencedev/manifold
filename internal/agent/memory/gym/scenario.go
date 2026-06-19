package gym

import (
	"manifold/internal/agent/memory"
	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/agent/memory/magma"
	"manifold/internal/policy"
)

// Subsystem names the memory subsystem a scenario primarily exercises.
type Subsystem string

const (
	SubsystemBelief      Subsystem = "belief"
	SubsystemEvolving    Subsystem = "evolving"
	SubsystemMagma       Subsystem = "magma"
	SubsystemDecision    Subsystem = "decision_lane"
	SubsystemArchaeology Subsystem = "archaeology"
	SubsystemRuntime     Subsystem = "runtime_fusion"
)

// Scenario is one ground-truth exercise of a memory subsystem. Scenarios are
// deterministic: seeded state plus a fixed knob set must always produce the
// same expectations.
type Scenario struct {
	ID          string
	Subsystem   Subsystem
	Description string
	// Knobs lists the configuration paths this scenario exercises, so Phase 2
	// can attribute pass/fail deltas to specific configuration dimensions.
	Knobs []string
	// Mutate pins scenario-specific knob overrides on top of the run's base
	// knob set. Scenarios that validate non-default config points use this.
	Mutate func(*Knobs)
	Seed   Seed
	Steps  []Step
}

// Seed is the deterministic state installed before any step runs.
type Seed struct {
	TenantID    int64
	Scopes      []belief.Scope
	Beliefs     []belief.Belief
	Decisions   []decision.Decision
	Assumptions []AssumptionSeed
	Episodes    []EvolvingEpisode
	MagmaEvents []magma.IngestRequest
	Policies    []policy.Record
}

// AssumptionSeed links a seeded decision (matched by statement substring) to
// a seeded belief.
type AssumptionSeed struct {
	DecisionStatement string
	BeliefID          string
	Criticality       decision.AssumptionCriticality
	StatementAtLink   string
	ConfidenceAtLink  float64
}

// EvolvingEpisode is one experience recorded into evolving memory.
type EvolvingEpisode struct {
	Input    string
	Output   string
	Feedback string
}

// Step is a tagged union; exactly one probe must be set.
type Step struct {
	Name          string
	Prompt        *PromptProbe
	EvolvingQuery *EvolvingSearchProbe
	EvolvingState *EvolvingStateProbe
	Distill       *DistillProbe
	BeliefChange  *BeliefChangeProbe
	Reconstruct   *ReconstructProbe
}

// PromptProbe runs Runtime.PrepareContext and checks the fused prompt block
// plus per-lane diagnostics.
type PromptProbe struct {
	Request memory.Request
	Expect  PromptExpect
}

// PromptExpect is the ground truth for one PrepareContext call.
type PromptExpect struct {
	MustContain    []string
	MustNotContain []string
	// Ordered snippets must each appear, in the given order.
	Ordered []string
	// LaneReturned asserts whether a named lane contributed text.
	LaneReturned map[string]bool
	// LaneItems asserts the exact item count a lane reported.
	LaneItems map[string]int
	// LaneItemsMax asserts an upper bound on a lane's item count.
	LaneItemsMax map[string]int
	// Truncated, when set, asserts the block truncation flag.
	Truncated *bool
	// MaxTokenEstimate, when > 0, asserts block.TokenEstimate <= the bound.
	MaxTokenEstimate int
}

// EvolvingSearchProbe checks evolving-memory retrieval and its diagnostics
// directly, which makes threshold and mode effects observable.
type EvolvingSearchProbe struct {
	Query string
	// ExpectResultCount, when non-nil, asserts the exact result count.
	ExpectResultCount *int
	// ExpectMode, when set, asserts the retrieval mode (hybrid/vector/keyword).
	ExpectMode string
	// ExpectVectorFilteredMin asserts at least N dense candidates were
	// filtered by the similarity threshold.
	ExpectVectorFilteredMin int
	// ExpectInputsContain: each substring must match some result's Input.
	ExpectInputsContain []string
	// ExpectInputsAbsent: no result's Input may contain these substrings.
	ExpectInputsAbsent []string
}

// EvolvingStateProbe checks the stored evolving-memory state (write-path
// ground truth: smart prune, FIFO caps).
type EvolvingStateProbe struct {
	ExpectEntryCount *int
	// ExpectInputsPresent: each substring must match some stored entry Input.
	ExpectInputsPresent []string
	// ExpectInputsAbsent: no stored entry Input may contain these substrings.
	ExpectInputsAbsent []string
}

// DistillProbe runs the deterministic SimpleDistiller over an episode
// envelope and (optionally) the deterministic auto-activation policy.
type DistillProbe struct {
	Input                decision.DistillationInput
	ExpectCandidateCount int
	// RecordCandidates persists distilled candidates to the decision store.
	RecordCandidates bool
	// AutoAccept runs Service.AutoAcceptCandidates over the recorded
	// candidates (a no-op when archaeology.auto_activate.enabled is false).
	AutoAccept           bool
	ExpectAcceptedCount  int
	ExpectReasonContains []string
	// ExpectDecisionStatus: decision statement substring -> expected status.
	ExpectDecisionStatus map[string]decision.DecisionStatus
	// ExpectDecisionReviewState: statement substring -> expected review state.
	ExpectDecisionReviewState map[string]decision.ReviewState
	// ExpectCandidateValidation: candidate statement substring -> validation status.
	ExpectCandidateValidation map[string]decision.CandidateValidationStatus
}

// BeliefChangeProbe fires one belief change event at the decision reactor and
// checks the resulting decision lifecycle state.
type BeliefChangeProbe struct {
	Event belief.ChangeEvent
	// UpsertAfter also persists Event.After so archaeology reconstruction
	// sees the new belief state.
	UpsertAfter               bool
	ExpectDecisionStatus      map[string]decision.DecisionStatus
	ExpectDecisionReviewState map[string]decision.ReviewState
}

// ReconstructProbe runs archaeology reconstruction and checks verdicts.
type ReconstructProbe struct {
	Query          string
	ScopeIDs       []string
	IncludeStale   bool
	ExpectVerdicts map[string]string
}

type Check struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// ScenarioResult is the machine-readable outcome for one scenario run.
type ScenarioResult struct {
	ScenarioID  string         `json:"scenarioId"`
	Subsystem   Subsystem      `json:"subsystem"`
	Description string         `json:"description,omitempty"`
	Knobs       []string       `json:"knobs"`
	KnobValues  map[string]any `json:"knobValues"`
	Checks      []Check        `json:"checks"`
	Passed      int            `json:"passed"`
	Failed      int            `json:"failed"`
	Error       string         `json:"error,omitempty"`
}

// SuiteResult aggregates one full gym run under one base knob set.
type SuiteResult struct {
	Name       string           `json:"name"`
	KnobValues map[string]any   `json:"knobValues"`
	Scenarios  []ScenarioResult `json:"scenarios"`
	Passed     int              `json:"passed"`
	Failed     int              `json:"failed"`
}
