package gym

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ScorecardSchemaVersion identifies the Phase 2 scorecard JSON schema.
// Bump this when the structure changes so Transit consumers can validate.
const ScorecardSchemaVersion = "memgym-scorecard/v1"

// JudgeModel is the frontier specialist that produces the qualitative
// verdict attached to a scorecard. The judge NEVER participates in
// deterministic scoring: BuildScorecard is pure Go over SuiteResult data,
// and the judge verdict is attached afterwards as data via WithJudge.
const JudgeModel = "claude_fable_5"

// Composite weighting: the deterministic pass rate dominates (ground truth
// is authoritative); the judge differentiates configurations that already
// satisfy ground truth. compositeScore = 70*passRate + 0.3*judgeOverall,
// clamped to [0,100]. Without a judge verdict the composite is 70*passRate.
const (
	compositeDeterministicWeight = 70.0
	compositeJudgeWeight         = 0.3
)

// GroupScore aggregates deterministic check outcomes for one grouping
// (a subsystem or a knob tag).
type GroupScore struct {
	Scenarios int     `json:"scenarios"`
	Checks    int     `json:"checks"`
	Passed    int     `json:"passed"`
	Failed    int     `json:"failed"`
	Errors    int     `json:"errors"`
	PassRate  float64 `json:"passRate"`
}

// KnobScore attributes deterministic outcomes to one configuration path,
// including every distinct value the knob held across the scenarios that
// declared it (base value plus Scenario.Mutate pins).
type KnobScore struct {
	GroupScore
	// Values holds the distinct stringified knob values observed across
	// attributed scenarios, sorted for deterministic output.
	Values []string `json:"values"`
}

// FailureDetail surfaces one failed check (or scenario error) so the judge
// and the optimization loop can see exactly what regressed without
// re-reading the full suite result.
type FailureDetail struct {
	ScenarioID string   `json:"scenarioId"`
	Subsystem  string   `json:"subsystem"`
	Check      string   `json:"check,omitempty"`
	Detail     string   `json:"detail,omitempty"`
	Error      string   `json:"error,omitempty"`
	Knobs      []string `json:"knobs,omitempty"`
}

// Guardrails are the hard gates for Phase 3: a candidate configuration that
// violates deterministic ground truth is rejected regardless of judge score.
type Guardrails struct {
	NoScenarioErrors     bool    `json:"noScenarioErrors"`
	AllChecksPassed      bool    `json:"allChecksPassed"`
	MinSubsystemPassRate float64 `json:"minSubsystemPassRate"`
	Pass                 bool    `json:"pass"`
}

// JudgeVerdict is the qualitative layer produced by the frontier judge
// specialist. It is attached as data by the orchestrator; gym code never
// calls an LLM.
type JudgeVerdict struct {
	Model           string             `json:"model"`
	OverallScore    float64            `json:"overallScore"` // 0-100
	SubsystemScores map[string]float64 `json:"subsystemScores,omitempty"`
	Strengths       []string           `json:"strengths,omitempty"`
	Risks           []string           `json:"risks,omitempty"`
	Recommendations []string           `json:"recommendations,omitempty"`
	Summary         string             `json:"summary,omitempty"`
}

// Scorecard is the Phase 2 machine-readable scoring artifact for one gym
// run. It is written to Transit under manifold/memory-testing/results/<runN>.
type Scorecard struct {
	SchemaVersion string `json:"schemaVersion"`
	Run           string `json:"run"`
	GeneratedAt   string `json:"generatedAt"`
	SuiteName     string `json:"suiteName"`
	// KnobValues is the base knob set for the run (per-scenario pinned
	// overrides remain visible in KnobScore.Values).
	KnobValues map[string]any        `json:"knobValues"`
	Totals     GroupScore            `json:"totals"`
	Subsystems map[string]GroupScore `json:"subsystems"`
	Knobs      map[string]KnobScore  `json:"knobs"`
	Failures   []FailureDetail       `json:"failures,omitempty"`
	Guardrails Guardrails            `json:"guardrails"`
	Judge      *JudgeVerdict         `json:"judge,omitempty"`
	// CompositeScore ranks configurations: 70*passRate + 0.3*judgeOverall.
	CompositeScore float64 `json:"compositeScore"`
}

// BuildScorecard deterministically aggregates one suite result into a
// scorecard. It performs no I/O and no LLM calls; identical input always
// yields an identical scorecard apart from GeneratedAt.
func BuildScorecard(run string, suite SuiteResult) Scorecard {
	card := newScorecard(run, suite)
	knobValues := map[string]map[string]struct{}{}

	for _, sc := range suite.Scenarios {
		addScenarioToScorecard(&card, sc, knobValues)
	}

	finalizeScorecard(&card, knobValues)
	return card
}

func newScorecard(run string, suite SuiteResult) Scorecard {
	return Scorecard{
		SchemaVersion: ScorecardSchemaVersion,
		Run:           run,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SuiteName:     suite.Name,
		KnobValues:    suite.KnobValues,
		Subsystems:    map[string]GroupScore{},
		Knobs:         map[string]KnobScore{},
	}
}

func addScenarioToScorecard(card *Scorecard, sc ScenarioResult, knobValues map[string]map[string]struct{}) {
	hadError := sc.Error != ""

	card.Totals.Scenarios++
	card.Totals.Checks += len(sc.Checks)
	card.Totals.Passed += sc.Passed
	card.Totals.Failed += sc.Failed
	if hadError {
		card.Totals.Errors++
		card.Failures = append(card.Failures, scenarioErrorFailure(sc))
	}

	addSubsystemScore(card, sc, hadError)
	addKnobScores(card, sc, hadError, knobValues)
	addCheckFailures(card, sc)
}

func addSubsystemScore(card *Scorecard, sc ScenarioResult, hadError bool) {
	sub := card.Subsystems[string(sc.Subsystem)]
	sub.Scenarios++
	sub.Checks += len(sc.Checks)
	sub.Passed += sc.Passed
	sub.Failed += sc.Failed
	if hadError {
		sub.Errors++
	}
	card.Subsystems[string(sc.Subsystem)] = sub
}

func addKnobScores(card *Scorecard, sc ScenarioResult, hadError bool, knobValues map[string]map[string]struct{}) {
	for _, tag := range sc.Knobs {
		ks := card.Knobs[tag]
		ks.Scenarios++
		ks.Checks += len(sc.Checks)
		ks.Passed += sc.Passed
		ks.Failed += sc.Failed
		if hadError {
			ks.Errors++
		}
		card.Knobs[tag] = ks

		if knobValues[tag] == nil {
			knobValues[tag] = map[string]struct{}{}
		}
		if v, ok := sc.KnobValues[tag]; ok {
			knobValues[tag][fmt.Sprintf("%v", v)] = struct{}{}
		}
	}
}

func addCheckFailures(card *Scorecard, sc ScenarioResult) {
	for _, check := range sc.Checks {
		if check.Pass {
			continue
		}
		card.Failures = append(card.Failures, FailureDetail{
			ScenarioID: sc.ScenarioID,
			Subsystem:  string(sc.Subsystem),
			Check:      check.Name,
			Detail:     check.Detail,
			Knobs:      sc.Knobs,
		})
	}
}

func scenarioErrorFailure(sc ScenarioResult) FailureDetail {
	return FailureDetail{
		ScenarioID: sc.ScenarioID,
		Subsystem:  string(sc.Subsystem),
		Error:      sc.Error,
		Knobs:      sc.Knobs,
	}
}

func finalizeScorecard(card *Scorecard, knobValues map[string]map[string]struct{}) {
	card.Totals.PassRate = passRate(card.Totals)
	for name, sub := range card.Subsystems {
		sub.PassRate = passRate(sub)
		card.Subsystems[name] = sub
	}
	for tag, ks := range card.Knobs {
		ks.PassRate = passRate(ks.GroupScore)
		vals := make([]string, 0, len(knobValues[tag]))
		for v := range knobValues[tag] {
			vals = append(vals, v)
		}
		sort.Strings(vals)
		ks.Values = vals
		card.Knobs[tag] = ks
	}

	minSubsystem := 1.0
	for _, sub := range card.Subsystems {
		if sub.PassRate < minSubsystem {
			minSubsystem = sub.PassRate
		}
	}
	if len(card.Subsystems) == 0 {
		minSubsystem = 0
	}
	card.Guardrails = Guardrails{
		NoScenarioErrors:     card.Totals.Errors == 0,
		AllChecksPassed:      card.Totals.Failed == 0 && card.Totals.Checks > 0,
		MinSubsystemPassRate: minSubsystem,
	}
	card.Guardrails.Pass = card.Guardrails.NoScenarioErrors && card.Guardrails.AllChecksPassed
	card.CompositeScore = composite(card.Totals.PassRate, nil)
}

// WithJudge attaches a judge verdict and recomputes the composite score.
// The deterministic sections are never altered by the judge.
func (c Scorecard) WithJudge(v JudgeVerdict) Scorecard {
	verdict := v
	if verdict.Model == "" {
		verdict.Model = JudgeModel
	}
	c.Judge = &verdict
	c.CompositeScore = composite(c.Totals.PassRate, c.Judge)
	return c
}

func passRate(g GroupScore) float64 {
	if g.Checks == 0 {
		return 0
	}
	return round4(float64(g.Passed) / float64(g.Checks))
}

func composite(detPassRate float64, judge *JudgeVerdict) float64 {
	score := compositeDeterministicWeight * detPassRate
	if judge != nil {
		score += compositeJudgeWeight * judge.OverallScore
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return round4(score)
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
