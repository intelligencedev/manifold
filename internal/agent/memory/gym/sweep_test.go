package gym

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fakeSuite fabricates a SuiteResult with the given number of failed checks
// out of total, attributed to one synthetic scenario.
func fakeSuite(knobs Knobs, total, failed int) SuiteResult {
	checks := make([]Check, 0, total)
	for i := 0; i < total; i++ {
		checks = append(checks, Check{Name: "c", Pass: i >= failed})
	}
	return SuiteResult{
		Name:       "fake",
		KnobValues: knobs.Values(),
		Scenarios: []ScenarioResult{{
			ScenarioID: "fake-scenario",
			Subsystem:  "belief",
			Knobs:      []string{KnobMagmaSemanticTopK},
			KnobValues: knobs.Values(),
			Checks:     checks,
			Passed:     total - failed,
			Failed:     failed,
		}},
		Passed: total - failed,
		Failed: failed,
	}
}

func TestSweepRejectsGuardrailFailuresAndKeepsIncumbentOnTies(t *testing.T) {
	dims := []Dimension{
		{ConfigPath: KnobMagmaSemanticTopK, Candidates: []Candidate{
			// Fails guardrails -> must be rejected even though swept.
			{Label: "10", Apply: func(k *Knobs) { k.MagmaSemanticTopK = 10 }},
			// Passes guardrails but ties deterministically -> incumbent wins.
			{Label: "40", Apply: func(k *Knobs) { k.MagmaSemanticTopK = 40 }},
		}},
	}
	eval := func(_ context.Context, _ string, k Knobs) SuiteResult {
		if k.MagmaSemanticTopK == 10 {
			return fakeSuite(k, 10, 2)
		}
		return fakeSuite(k, 10, 0)
	}
	res, err := Sweep(context.Background(), SweepRequest{
		SuiteName:    "t",
		Base:         DefaultKnobs(),
		Dimensions:   dims,
		Evaluator:    eval,
		FirstRun:     2,
		MaxFinalists: 5,
	})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.BaseRun != "run2" {
		t.Fatalf("base run = %s, want run2", res.BaseRun)
	}
	if res.WinnerRun != "run2" {
		t.Fatalf("winner = %s, want incumbent base run2", res.WinnerRun)
	}
	if got := res.Dimensions[0].AcceptedValue; got != "20" {
		t.Fatalf("accepted value = %s, want incumbent 20", got)
	}
	if got := res.WinningKnobValues[KnobMagmaSemanticTopK]; got != 20 {
		t.Fatalf("winning topK = %v, want 20", got)
	}
	// run3 (topK=10) failed guardrails; run4 (topK=40) tied.
	for _, o := range res.Outcomes {
		switch o.Run {
		case "run3":
			if o.GuardrailsPass || o.Accepted {
				t.Fatalf("run3 must be rejected on guardrails: %+v", o)
			}
		case "run4":
			if !o.GuardrailsPass || o.Accepted || !strings.Contains(o.Reason, "tie") {
				t.Fatalf("run4 must be a rejected tie: %+v", o)
			}
		}
	}
	// The tied guardrail-passing candidate is pooled as a judge finalist.
	if len(res.Finalists) != 2 || res.Finalists[0] != "run2" || res.Finalists[1] != "run4" {
		t.Fatalf("finalists = %v, want [run2 run4]", res.Finalists)
	}
}

func TestSweepGuardrailFailureNeverCarriesForward(t *testing.T) {
	dims := []Dimension{
		{ConfigPath: KnobMagmaSemanticTopK, Candidates: []Candidate{
			{Label: "40", Apply: func(k *Knobs) { k.MagmaSemanticTopK = 40 }},
		}},
		{ConfigPath: KnobEvolvingMaxSize, Candidates: []Candidate{
			{Label: "500", Apply: func(k *Knobs) { k.EvolvingMaxSize = 500 }},
		}},
	}
	eval := func(_ context.Context, _ string, k Knobs) SuiteResult {
		if k.EvolvingMaxSize == 500 {
			return fakeSuite(k, 10, 1) // guardrail failure -> rejected
		}
		return fakeSuite(k, 10, 0)
	}
	res, err := Sweep(context.Background(), SweepRequest{
		SuiteName:    "t",
		Base:         DefaultKnobs(),
		Dimensions:   dims,
		Evaluator:    eval,
		FirstRun:     1,
		MaxFinalists: 5,
	})
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	// Base run1 passes 10/10. run2 (topK=40) ties -> incumbent retained.
	// run3 (maxSize=500) fails guardrails -> rejected, defaults retained.
	if res.WinnerRun != "run1" {
		t.Fatalf("winner = %s, want run1", res.WinnerRun)
	}
	if got := res.WinningKnobValues[KnobEvolvingMaxSize]; got != 2000 {
		t.Fatalf("winning maxSize = %v, want 2000", got)
	}
	if got := res.Dimensions[1].AcceptedValue; got != "2000" {
		t.Fatalf("dimension accepted value = %s, want incumbent 2000", got)
	}
}

func TestSweepRefusesPinnedDimensionsAndPinViolations(t *testing.T) {
	eval := func(_ context.Context, _ string, k Knobs) SuiteResult { return fakeSuite(k, 3, 0) }

	// Sweeping a pinned path must be refused outright.
	pinnedDim := []Dimension{{ConfigPath: KnobAutoActivateEnabled, Candidates: []Candidate{
		{Label: "true", Apply: func(k *Knobs) { k.AutoActivateEnabled = true }},
	}}}
	if _, err := Sweep(context.Background(), SweepRequest{
		SuiteName:    "t",
		Base:         DefaultKnobs(),
		Dimensions:   pinnedDim,
		Evaluator:    eval,
		FirstRun:     1,
		MaxFinalists: 3,
	}); err == nil {
		t.Fatal("expected error sweeping a safety-pinned dimension")
	}

	// A candidate that mutates a pin as a side effect must abort the sweep.
	sideEffect := []Dimension{{ConfigPath: KnobMemTimeoutMs, Candidates: []Candidate{
		{Label: "200", Apply: func(k *Knobs) {
			k.TimeoutMs = 200
			k.BeliefIncludeContradictions = false
		}},
	}}}
	if _, err := Sweep(context.Background(), SweepRequest{
		SuiteName:    "t",
		Base:         DefaultKnobs(),
		Dimensions:   sideEffect,
		Evaluator:    eval,
		FirstRun:     1,
		MaxFinalists: 3,
	}); err == nil {
		t.Fatal("expected error when a candidate violates a safety pin")
	}

	// A base config violating a pin must be refused.
	badBase := DefaultKnobs()
	badBase.AutoActivateEnabled = true
	if _, err := Sweep(context.Background(), SweepRequest{
		SuiteName:    "t",
		Base:         badBase,
		Evaluator:    eval,
		FirstRun:     1,
		MaxFinalists: 3,
	}); err == nil {
		t.Fatal("expected error for pin-violating base config")
	}
}

func TestPhase3DimensionsExcludePinnedPathsAndMatchPlan(t *testing.T) {
	dims := Phase3Dimensions()
	if len(dims) != 13 {
		t.Fatalf("expected 13 dimensions, got %d", len(dims))
	}
	pinned := map[string]bool{
		KnobAutoActivateEnabled:         true,
		KnobAutoActivateMinConfidence:   true,
		KnobBeliefIncludeContradictions: true,
	}
	base := DefaultKnobs()
	for _, dim := range dims {
		if pinned[dim.ConfigPath] {
			t.Fatalf("pinned path %s must not be swept", dim.ConfigPath)
		}
		if _, ok := base.Values()[dim.ConfigPath]; !ok {
			t.Fatalf("dimension %s does not map to a known knob path", dim.ConfigPath)
		}
		for _, cand := range dim.Candidates {
			knobs := base
			cand.Apply(&knobs)
			if err := verifyPins(knobs); err != nil {
				t.Fatalf("candidate %s=%s violates pins: %v", dim.ConfigPath, cand.Label, err)
			}
			got := fmt.Sprintf("%v", knobs.Values()[dim.ConfigPath])
			if got != cand.Label {
				t.Fatalf("candidate %s: Apply produced %v, label %s", dim.ConfigPath, got, cand.Label)
			}
			if fmt.Sprintf("%v", base.Values()[dim.ConfigPath]) == cand.Label {
				t.Fatalf("candidate %s=%s duplicates the default incumbent", dim.ConfigPath, cand.Label)
			}
		}
	}
}
