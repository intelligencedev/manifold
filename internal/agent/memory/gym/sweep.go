package gym

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// SweepSchemaVersion identifies the Phase 3 sweep-summary JSON schema.
const SweepSchemaVersion = "memgym-sweep/v1"

// pinnedPaths are the safety-critical knobs Phase 3 must never mutate.
// Relaxing any of them requires an explicit operator decision (see master
// plan key manifold/memory-testing/plan).
var pinnedPaths = []string{
	KnobAutoActivateEnabled,
	KnobAutoActivateMinConfidence,
	KnobBeliefIncludeContradictions,
}

// Candidate is one swept value for a dimension. Apply mutates a copy of the
// current knob set; Label is the stringified value used in reports.
type Candidate struct {
	Label string
	Apply func(*Knobs)
}

// Dimension is one coordinate-descent axis: a single configuration path and
// the candidate values to evaluate against the incumbent.
type Dimension struct {
	ConfigPath string
	Candidates []Candidate
}

// Evaluator produces a SuiteResult for one candidate knob set. The default
// SuiteEvaluator wraps RunSuite; tests may substitute fakes.
type Evaluator func(ctx context.Context, run string, knobs Knobs) SuiteResult

// SuiteEvaluator wraps RunSuite over a fixed scenario list.
func SuiteEvaluator(suiteName string, scenarios []Scenario) Evaluator {
	return func(ctx context.Context, run string, knobs Knobs) SuiteResult {
		return RunSuite(ctx, suiteName+"/"+run, scenarios, knobs)
	}
}

// Persist receives every evaluated run so the caller can write artifacts.
// It must not alter the sweep outcome.
type Persist func(run string, suite SuiteResult, card Scorecard) error

// CandidateOutcome summarizes one evaluated configuration.
type CandidateOutcome struct {
	Run            string  `json:"run"`
	ConfigPath     string  `json:"configPath,omitempty"`
	Value          string  `json:"value,omitempty"`
	Incumbent      string  `json:"incumbent,omitempty"`
	GuardrailsPass bool    `json:"guardrailsPass"`
	PassRate       float64 `json:"passRate"`
	Checks         int     `json:"checks"`
	Failed         int     `json:"failed"`
	Errors         int     `json:"errors"`
	Accepted       bool    `json:"accepted"`
	Reason         string  `json:"reason"`
}

// DimensionOutcome records the coordinate-descent verdict for one axis.
type DimensionOutcome struct {
	ConfigPath     string   `json:"configPath"`
	Incumbent      string   `json:"incumbent"`
	AcceptedValue  string   `json:"acceptedValue"`
	RejectedValues []string `json:"rejectedValues,omitempty"`
	Runs           []string `json:"runs"`
}

// SweepResult is the Phase 3 sweep summary artifact.
type SweepResult struct {
	SchemaVersion     string             `json:"schemaVersion"`
	GeneratedAt       string             `json:"generatedAt"`
	SuiteName         string             `json:"suiteName"`
	BaseRun           string             `json:"baseRun"`
	Pinned            map[string]any     `json:"pinned"`
	Outcomes          []CandidateOutcome `json:"outcomes"`
	Dimensions        []DimensionOutcome `json:"dimensions"`
	WinnerRun         string             `json:"winnerRun"`
	WinningKnobValues map[string]any     `json:"winningKnobValues"`
	// Finalists are the deterministic shortlist for the frontier judge:
	// the winner first, then guardrail-passing single-dimension candidates
	// (at most one per dimension, in judge-priority dimension order).
	Finalists []string `json:"finalists"`
}

// SweepRequest groups the coordinate-descent inputs for one sweep run.
type SweepRequest struct {
	SuiteName    string
	Base         Knobs
	Dimensions   []Dimension
	Evaluator    Evaluator
	Persist      Persist
	FirstRun     int
	MaxFinalists int
}

type finalistPoolEntry struct {
	run      string
	passRate float64
}

type sweepRunner struct {
	ctx           context.Context
	req           SweepRequest
	res           SweepResult
	current       Knobs
	incumbentRate float64
	runN          int
	finalistPool  []finalistPoolEntry
}

// Phase3Dimensions is the judge-directed Phase 3 search space, in coordinate
// order MAGMA -> evolving -> archaeology reactor/budgets -> retrieval
// robustness. Defaults (the incumbents) are not repeated as candidates.
// Safety-critical knobs are intentionally absent (see pinnedPaths).
func Phase3Dimensions() []Dimension {
	f := func(label string, apply func(*Knobs)) Candidate {
		return Candidate{Label: label, Apply: apply}
	}
	return []Dimension{
		{ConfigPath: KnobMagmaSimilarityThreshold, Candidates: []Candidate{
			f("0.6", func(k *Knobs) { k.MagmaSimilarityThreshold = 0.6 }),
			f("0.8", func(k *Knobs) { k.MagmaSimilarityThreshold = 0.8 }),
		}},
		{ConfigPath: KnobMagmaSemanticTopK, Candidates: []Candidate{
			f("10", func(k *Knobs) { k.MagmaSemanticTopK = 10 }),
			f("40", func(k *Knobs) { k.MagmaSemanticTopK = 40 }),
		}},
		{ConfigPath: KnobMagmaIntentClassification, Candidates: []Candidate{
			f("keyword", func(k *Knobs) { k.MagmaIntentClassification = "keyword" }),
			f("llm", func(k *Knobs) { k.MagmaIntentClassification = "llm" }),
		}},
		{ConfigPath: KnobEvolvingMaxSize, Candidates: []Candidate{
			f("500", func(k *Knobs) { k.EvolvingMaxSize = 500 }),
			f("5000", func(k *Knobs) { k.EvolvingMaxSize = 5000 }),
		}},
		{ConfigPath: KnobEvolvingRelevanceDecay, Candidates: []Candidate{
			f("0.95", func(k *Knobs) { k.EvolvingRelevanceDecay = 0.95 }),
		}},
		{ConfigPath: KnobEvolvingMinRelevance, Candidates: []Candidate{
			f("0.2", func(k *Knobs) { k.EvolvingMinRelevance = 0.2 }),
		}},
		{ConfigPath: KnobEvolvingEnableRAG, Candidates: []Candidate{
			f("false", func(k *Knobs) { k.EvolvingEnableRAG = false }),
		}},
		{ConfigPath: KnobReactorConfidenceFloor, Candidates: []Candidate{
			f("0.25", func(k *Knobs) { k.ReactorConfidenceFloor = 0.25 }),
			f("0.5", func(k *Knobs) { k.ReactorConfidenceFloor = 0.5 }),
		}},
		{ConfigPath: KnobDecisionTokenBudget, Candidates: []Candidate{
			f("300", func(k *Knobs) { k.DecisionTokenBudget = 300 }),
			f("900", func(k *Knobs) { k.DecisionTokenBudget = 900 }),
		}},
		{ConfigPath: KnobMemMaxTokensPerPrompt, Candidates: []Candidate{
			f("1500", func(k *Knobs) { k.MaxTokensPerPrompt = 1500 }),
			f("3000", func(k *Knobs) { k.MaxTokensPerPrompt = 3000 }),
		}},
		{ConfigPath: KnobBeliefMaxTokensPerPrompt, Candidates: []Candidate{
			f("400", func(k *Knobs) { k.BeliefMaxTokensPerPrompt = 400 }),
		}},
		{ConfigPath: KnobMemIncludeRecent, Candidates: []Candidate{
			f("false", func(k *Knobs) { k.IncludeRecent = false }),
		}},
		{ConfigPath: KnobMemTimeoutMs, Candidates: []Candidate{
			f("200", func(k *Knobs) { k.TimeoutMs = 200 }),
		}},
	}
}

// Sweep runs deterministic coordinate descent: one dimension at a time, each
// candidate evaluated with the gym suite and scored deterministically.
//
// Rules (Phase 3 hard gates):
//   - guardrails.pass=false rejects a candidate unconditionally.
//   - a candidate replaces the incumbent only when its deterministic pass
//     rate is strictly higher; ties keep the incumbent (defaults win ties).
//   - safety-critical pins are enforced before and after every mutation.
//
// firstRun numbers the artifacts: base = run<firstRun>, candidates follow
// sequentially. maxFinalists caps the judge shortlist (winner included).
func Sweep(ctx context.Context, req SweepRequest) (SweepResult, error) {
	runner, err := newSweepRunner(ctx, req)
	if err != nil {
		return SweepResult{}, err
	}
	return runner.run()
}

func newSweepRunner(ctx context.Context, req SweepRequest) (*sweepRunner, error) {
	if req.Evaluator == nil {
		return nil, fmt.Errorf("sweep: evaluator is required")
	}
	if req.FirstRun < 1 {
		req.FirstRun = 1
	}
	if req.MaxFinalists < 1 {
		req.MaxFinalists = 1
	}
	if err := verifyPins(req.Base); err != nil {
		return nil, fmt.Errorf("sweep: base config violates safety pin: %w", err)
	}
	if err := verifySweepDimensions(req.Dimensions); err != nil {
		return nil, err
	}
	res := SweepResult{
		SchemaVersion: SweepSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SuiteName:     req.SuiteName,
		Pinned:        map[string]any{},
	}
	for _, pin := range pinnedPaths {
		res.Pinned[pin] = req.Base.Values()[pin]
	}
	return &sweepRunner{
		ctx:          ctx,
		req:          req,
		res:          res,
		current:      req.Base,
		runN:         req.FirstRun,
		finalistPool: make([]finalistPoolEntry, 0, len(req.Dimensions)),
	}, nil
}

func verifySweepDimensions(dims []Dimension) error {
	for _, dim := range dims {
		for _, pin := range pinnedPaths {
			if dim.ConfigPath == pin {
				return fmt.Errorf("sweep: dimension %s is safety-pinned and may not be swept", pin)
			}
		}
	}
	return nil
}

func (r *sweepRunner) run() (SweepResult, error) {
	if err := r.evaluateBase(); err != nil {
		return r.res, err
	}
	for _, dim := range r.req.Dimensions {
		if err := r.evaluateDimension(dim); err != nil {
			return r.res, err
		}
	}
	r.finish()
	return r.res, nil
}

func (r *sweepRunner) nextRun() string {
	run := fmt.Sprintf("run%d", r.runN)
	r.runN++
	return run
}

func (r *sweepRunner) evaluate(run string, knobs Knobs) (Scorecard, error) {
	suite := r.req.Evaluator(r.ctx, run, knobs)
	card := BuildScorecard(run, suite)
	if r.req.Persist != nil {
		if err := r.req.Persist(run, suite, card); err != nil {
			return Scorecard{}, fmt.Errorf("persist %s: %w", run, err)
		}
	}
	return card, nil
}

func (r *sweepRunner) evaluateBase() error {
	r.res.BaseRun = r.nextRun()
	baseCard, err := r.evaluate(r.res.BaseRun, r.current)
	if err != nil {
		return err
	}
	r.res.Outcomes = append(r.res.Outcomes, outcome(outcomeInput{
		run:      r.res.BaseRun,
		card:     baseCard,
		reason:   guardrailReason(baseCard, "base configuration"),
		accepted: false,
	}))
	if !baseCard.Guardrails.Pass {
		return fmt.Errorf("sweep: base configuration fails guardrails; aborting (run %s)", r.res.BaseRun)
	}
	r.incumbentRate = baseCard.Totals.PassRate
	r.res.WinnerRun = r.res.BaseRun
	return nil
}

func (r *sweepRunner) evaluateDimension(dim Dimension) error {
	incumbentLabel := fmt.Sprintf("%v", r.current.Values()[dim.ConfigPath])
	state := dimensionState{
		outcome: DimensionOutcome{
			ConfigPath:    dim.ConfigPath,
			Incumbent:     incumbentLabel,
			AcceptedValue: incumbentLabel,
		},
		bestIdx:  -1,
		bestRate: r.incumbentRate,
		cards:    make([]Scorecard, len(dim.Candidates)),
		runs:     make([]string, len(dim.Candidates)),
	}

	for i, cand := range dim.Candidates {
		if err := r.evaluateCandidate(dim, cand, i, &state); err != nil {
			return err
		}
	}
	r.recordDimension(dim, incumbentLabel, state)
	return nil
}

type dimensionState struct {
	outcome  DimensionOutcome
	bestIdx  int
	bestRate float64
	cards    []Scorecard
	runs     []string
}

func (r *sweepRunner) evaluateCandidate(dim Dimension, cand Candidate, idx int, state *dimensionState) error {
	knobs := r.current
	cand.Apply(&knobs)
	if err := verifyPins(knobs); err != nil {
		return fmt.Errorf("sweep: candidate %s=%s violates safety pin: %w", dim.ConfigPath, cand.Label, err)
	}
	run := r.nextRun()
	state.runs[idx] = run
	state.outcome.Runs = append(state.outcome.Runs, run)
	card, err := r.evaluate(run, knobs)
	if err != nil {
		return err
	}
	state.cards[idx] = card
	if card.Guardrails.Pass && card.Totals.PassRate > state.bestRate {
		state.bestRate = card.Totals.PassRate
		state.bestIdx = idx
	}
	return nil
}

func (r *sweepRunner) recordDimension(dim Dimension, incumbentLabel string, state dimensionState) {
	for i, cand := range dim.Candidates {
		card := state.cards[i]
		accepted := i == state.bestIdx
		r.res.Outcomes = append(r.res.Outcomes, outcome(outcomeInput{
			run:       state.runs[i],
			path:      dim.ConfigPath,
			value:     cand.Label,
			incumbent: incumbentLabel,
			card:      card,
			accepted:  accepted,
			reason:    candidateReason(card, accepted, r.incumbentRate),
		}))
		if !accepted {
			state.outcome.RejectedValues = append(state.outcome.RejectedValues, cand.Label)
		}
	}
	if state.bestIdx >= 0 {
		dim.Candidates[state.bestIdx].Apply(&r.current)
		r.incumbentRate = state.bestRate
		state.outcome.AcceptedValue = dim.Candidates[state.bestIdx].Label
		r.res.WinnerRun = state.runs[state.bestIdx]
	} else if entry, ok := bestRejectedFinalist(state); ok {
		r.finalistPool = append(r.finalistPool, entry)
	}
	r.res.Dimensions = append(r.res.Dimensions, state.outcome)
}

func bestRejectedFinalist(state dimensionState) (finalistPoolEntry, bool) {
	poolIdx := -1
	poolRate := -1.0
	for i, card := range state.cards {
		if card.Guardrails.Pass && card.Totals.PassRate > poolRate {
			poolRate = card.Totals.PassRate
			poolIdx = i
		}
	}
	if poolIdx < 0 {
		return finalistPoolEntry{}, false
	}
	return finalistPoolEntry{run: state.runs[poolIdx], passRate: poolRate}, true
}

func (r *sweepRunner) finish() {
	r.res.WinningKnobValues = r.current.Values()
	r.res.Finalists = []string{r.res.WinnerRun}
	sort.SliceStable(r.finalistPool, func(i, j int) bool {
		return r.finalistPool[i].passRate > r.finalistPool[j].passRate
	})
	for _, entry := range r.finalistPool {
		if len(r.res.Finalists) >= r.req.MaxFinalists {
			break
		}
		if entry.run != r.res.WinnerRun {
			r.res.Finalists = append(r.res.Finalists, entry.run)
		}
	}
}

func verifyPins(k Knobs) error {
	if k.AutoActivateEnabled {
		return fmt.Errorf("%s must remain false", KnobAutoActivateEnabled)
	}
	if k.AutoActivateMinConfidence != 0.85 {
		return fmt.Errorf("%s must remain 0.85", KnobAutoActivateMinConfidence)
	}
	if !k.BeliefIncludeContradictions {
		return fmt.Errorf("%s must remain true", KnobBeliefIncludeContradictions)
	}
	return nil
}

type outcomeInput struct {
	run       string
	path      string
	value     string
	incumbent string
	card      Scorecard
	accepted  bool
	reason    string
}

func outcome(in outcomeInput) CandidateOutcome {
	return CandidateOutcome{
		Run:            in.run,
		ConfigPath:     in.path,
		Value:          in.value,
		Incumbent:      in.incumbent,
		GuardrailsPass: in.card.Guardrails.Pass,
		PassRate:       in.card.Totals.PassRate,
		Checks:         in.card.Totals.Checks,
		Failed:         in.card.Totals.Failed,
		Errors:         in.card.Totals.Errors,
		Accepted:       in.accepted,
		Reason:         in.reason,
	}
}

func guardrailReason(card Scorecard, label string) string {
	if card.Guardrails.Pass {
		return label + ": guardrails pass"
	}
	return fmt.Sprintf("%s: guardrails FAIL (%d failed checks, %d scenario errors)",
		label, card.Totals.Failed, card.Totals.Errors)
}

func candidateReason(card Scorecard, accepted bool, incumbentRate float64) string {
	switch {
	case !card.Guardrails.Pass:
		return fmt.Sprintf("rejected: guardrails FAIL (%d failed checks, %d scenario errors)",
			card.Totals.Failed, card.Totals.Errors)
	case accepted:
		return fmt.Sprintf("accepted: passRate %.4f > incumbent %.4f", card.Totals.PassRate, incumbentRate)
	case card.Totals.PassRate == incumbentRate:
		return "rejected: deterministic tie with incumbent (defaults win ties)"
	case card.Totals.PassRate < incumbentRate:
		return fmt.Sprintf("rejected: passRate %.4f < incumbent %.4f", card.Totals.PassRate, incumbentRate)
	default:
		return "rejected: another candidate in this dimension scored higher"
	}
}
