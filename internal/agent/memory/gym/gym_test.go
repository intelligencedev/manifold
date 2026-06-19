package gym

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestSuiteGroundTruth runs every scenario under the baseline knob set
// (config.yaml.example defaults plus per-scenario pinned overrides) and
// requires every deterministic check to pass. Set MEMORY_GYM_RESULTS to a
// file path to also persist the machine-readable suite result for Phase 2.
func TestSuiteGroundTruth(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	suite := RunSuite(ctx, "baseline", Suite(), DefaultKnobs())
	for _, scenario := range suite.Scenarios {
		t.Run(scenario.ScenarioID, func(t *testing.T) {
			if scenario.Error != "" {
				t.Fatalf("scenario error: %s", scenario.Error)
			}
			for _, check := range scenario.Checks {
				if !check.Pass {
					t.Errorf("check %s failed: %s", check.Name, check.Detail)
				}
			}
		})
	}
	if path := os.Getenv("MEMORY_GYM_RESULTS"); path != "" {
		if err := WriteResults(path, suite); err != nil {
			t.Fatalf("write results: %v", err)
		}
	}
}

// TestScenarioIDsUnique guards the suite against duplicate scenario IDs,
// which would corrupt Phase 2 score attribution.
func TestScenarioIDsUnique(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, sc := range Suite() {
		if sc.ID == "" {
			t.Fatal("scenario with empty ID")
		}
		if seen[sc.ID] {
			t.Fatalf("duplicate scenario ID %q", sc.ID)
		}
		seen[sc.ID] = true
	}
}

// TestScenarioKnobTagsResolve ensures every knob tag a scenario claims to
// exercise maps to a real configuration path in Knobs.Values().
func TestScenarioKnobTagsResolve(t *testing.T) {
	t.Parallel()
	values := DefaultKnobs().Values()
	for _, sc := range Suite() {
		if len(sc.Knobs) == 0 {
			t.Errorf("scenario %s declares no knobs", sc.ID)
			continue
		}
		for _, tag := range sc.Knobs {
			if _, ok := values[tag]; !ok {
				t.Errorf("scenario %s references unknown knob tag %q", sc.ID, tag)
			}
		}
	}
}

// TestSubsystemCoverage requires ground-truth scenarios for every memory
// subsystem in scope for the tuning loop.
func TestSubsystemCoverage(t *testing.T) {
	t.Parallel()
	want := []Subsystem{
		SubsystemBelief,
		SubsystemEvolving,
		SubsystemMagma,
		SubsystemDecision,
		SubsystemArchaeology,
		SubsystemRuntime,
	}
	counts := map[Subsystem]int{}
	for _, sc := range Suite() {
		counts[sc.Subsystem]++
	}
	for _, subsystem := range want {
		if counts[subsystem] == 0 {
			t.Errorf("no scenarios for subsystem %s", subsystem)
		}
	}
}
