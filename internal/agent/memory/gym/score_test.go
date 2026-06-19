package gym

import "testing"

func sampleSuite() SuiteResult {
	return SuiteResult{
		Name: "sample",
		KnobValues: map[string]any{
			KnobEvolvingTopK:         4,
			KnobBeliefMinConfidence:  0.35,
			KnobDecisionMaxPerPrompt: 5,
		},
		Scenarios: []ScenarioResult{
			{
				ScenarioID: "a",
				Subsystem:  SubsystemBelief,
				Knobs:      []string{KnobBeliefMinConfidence},
				KnobValues: map[string]any{KnobBeliefMinConfidence: 0.35},
				Checks: []Check{
					{Name: "c1", Pass: true},
					{Name: "c2", Pass: true},
				},
				Passed: 2,
			},
			{
				ScenarioID: "b",
				Subsystem:  SubsystemBelief,
				Knobs:      []string{KnobBeliefMinConfidence},
				KnobValues: map[string]any{KnobBeliefMinConfidence: 0.2},
				Checks: []Check{
					{Name: "c1", Pass: true},
					{Name: "c2", Pass: false, Detail: "missing snippet"},
				},
				Passed: 1,
				Failed: 1,
			},
			{
				ScenarioID: "c",
				Subsystem:  SubsystemEvolving,
				Knobs:      []string{KnobEvolvingTopK},
				KnobValues: map[string]any{KnobEvolvingTopK: 4},
				Checks: []Check{
					{Name: "c1", Pass: true},
				},
				Passed: 1,
			},
			{
				ScenarioID: "d",
				Subsystem:  SubsystemDecision,
				Knobs:      []string{KnobDecisionMaxPerPrompt},
				KnobValues: map[string]any{KnobDecisionMaxPerPrompt: 5},
				Error:      "boom",
			},
		},
		Passed: 4,
		Failed: 1,
	}
}

func TestBuildScorecardAggregation(t *testing.T) {
	t.Parallel()
	card := BuildScorecard("run-test", sampleSuite())

	if card.SchemaVersion != ScorecardSchemaVersion {
		t.Fatalf("schema version = %q", card.SchemaVersion)
	}
	if card.Totals.Scenarios != 4 || card.Totals.Checks != 5 ||
		card.Totals.Passed != 4 || card.Totals.Failed != 1 || card.Totals.Errors != 1 {
		t.Fatalf("totals = %+v", card.Totals)
	}
	if card.Totals.PassRate != 0.8 {
		t.Fatalf("pass rate = %v", card.Totals.PassRate)
	}

	belief := card.Subsystems[string(SubsystemBelief)]
	if belief.Scenarios != 2 || belief.Checks != 4 || belief.Passed != 3 || belief.Failed != 1 {
		t.Fatalf("belief subsystem = %+v", belief)
	}
	if belief.PassRate != 0.75 {
		t.Fatalf("belief pass rate = %v", belief.PassRate)
	}
	decisionSub := card.Subsystems[string(SubsystemDecision)]
	if decisionSub.Errors != 1 {
		t.Fatalf("decision subsystem errors = %+v", decisionSub)
	}

	knob := card.Knobs[KnobBeliefMinConfidence]
	if knob.Scenarios != 2 || knob.Failed != 1 {
		t.Fatalf("knob score = %+v", knob)
	}
	if len(knob.Values) != 2 || knob.Values[0] != "0.2" || knob.Values[1] != "0.35" {
		t.Fatalf("knob values = %v", knob.Values)
	}

	// 1 failed check + 1 scenario error = 2 failure details.
	if len(card.Failures) != 2 {
		t.Fatalf("failures = %+v", card.Failures)
	}

	if card.Guardrails.Pass {
		t.Fatal("guardrails must fail when checks fail or scenarios error")
	}
	if card.Guardrails.NoScenarioErrors || card.Guardrails.AllChecksPassed {
		t.Fatalf("guardrails = %+v", card.Guardrails)
	}

	// Composite without judge: 70 * 0.8 = 56.
	if card.CompositeScore != 56 {
		t.Fatalf("composite = %v", card.CompositeScore)
	}
}

func TestBuildScorecardAllPass(t *testing.T) {
	t.Parallel()
	suite := sampleSuite()
	suite.Scenarios = suite.Scenarios[:1] // only the fully passing scenario
	card := BuildScorecard("run-test", suite)

	if !card.Guardrails.Pass || !card.Guardrails.NoScenarioErrors || !card.Guardrails.AllChecksPassed {
		t.Fatalf("guardrails = %+v", card.Guardrails)
	}
	if card.Guardrails.MinSubsystemPassRate != 1 {
		t.Fatalf("min subsystem pass rate = %v", card.Guardrails.MinSubsystemPassRate)
	}
	if card.Totals.PassRate != 1 || card.CompositeScore != 70 {
		t.Fatalf("pass rate %v composite %v", card.Totals.PassRate, card.CompositeScore)
	}
	if len(card.Failures) != 0 {
		t.Fatalf("failures = %+v", card.Failures)
	}
}

func TestWithJudgeComposite(t *testing.T) {
	t.Parallel()
	suite := sampleSuite()
	suite.Scenarios = suite.Scenarios[:1]
	card := BuildScorecard("run-test", suite)

	judged := card.WithJudge(JudgeVerdict{OverallScore: 90})
	if judged.Judge == nil || judged.Judge.Model != JudgeModel {
		t.Fatalf("judge = %+v", judged.Judge)
	}
	// 70*1.0 + 0.3*90 = 97.
	if judged.CompositeScore != 97 {
		t.Fatalf("composite = %v", judged.CompositeScore)
	}
	// Deterministic sections unchanged.
	if judged.Totals != card.Totals {
		t.Fatalf("totals mutated: %+v vs %+v", judged.Totals, card.Totals)
	}
	// Original card untouched (value semantics for judge attachment).
	if card.Judge != nil {
		t.Fatal("WithJudge must not mutate the receiver's judge")
	}
}

func TestBaselineSuiteScorecard(t *testing.T) {
	t.Parallel()
	// End-to-end determinism: the real suite under default knobs must yield
	// a guardrail-passing scorecard with full knob/subsystem attribution.
	ctx := t.Context()
	suite := RunSuite(ctx, "baseline", Suite(), DefaultKnobs())
	card := BuildScorecard("baseline", suite)

	if !card.Guardrails.Pass {
		t.Fatalf("baseline guardrails failed: %+v failures=%+v", card.Guardrails, card.Failures)
	}
	if card.Totals.PassRate != 1 {
		t.Fatalf("baseline pass rate = %v", card.Totals.PassRate)
	}
	for _, subsystem := range []Subsystem{
		SubsystemBelief, SubsystemEvolving, SubsystemMagma,
		SubsystemDecision, SubsystemArchaeology, SubsystemRuntime,
	} {
		if _, ok := card.Subsystems[string(subsystem)]; !ok {
			t.Errorf("missing subsystem %s in scorecard", subsystem)
		}
	}
	if len(card.Knobs) == 0 {
		t.Fatal("no knob attribution in baseline scorecard")
	}
}
