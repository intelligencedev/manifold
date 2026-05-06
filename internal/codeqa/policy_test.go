package codeqa

import "testing"

func TestDecideRejectsHardFailures(t *testing.T) {
	aggregate := Decide(DecisionInput{
		Bundle:          DiffBundle{},
		Gates:           []GateResult{{Name: "go_build", Ref: "HEAD", HardFail: true}},
		Judges:          []JudgeVerdict{{Confidence: 0.95, Scores: map[string]float64{"correctness": 1}}},
		AcceptThreshold: 0.10,
		MinConfidence:   0.70,
	})
	if aggregate.Action != ActionReject {
		t.Fatalf("expected reject, got %s", aggregate.Action)
	}
	if len(aggregate.HardFailures) != 1 {
		t.Fatalf("expected hard failure to be preserved, got %v", aggregate.HardFailures)
	}
}

func TestDecideRoutesHighRiskToHumanReview(t *testing.T) {
	aggregate := Decide(DecisionInput{
		Bundle:          DiffBundle{Files: []ChangedFile{{Path: "internal/auth/session.go"}}},
		Judges:          []JudgeVerdict{{Confidence: 0.95, Scores: map[string]float64{"correctness": 1}}},
		AcceptThreshold: 0.10,
		MinConfidence:   0.70,
		HighRiskGlobs:   []string{"**/auth/**"},
	})
	if aggregate.Action != ActionHumanReview {
		t.Fatalf("expected human review, got %s", aggregate.Action)
	}
}

func TestDecideAcceptsWhenThresholdsMet(t *testing.T) {
	aggregate := Decide(DecisionInput{
		Bundle: DiffBundle{Files: []ChangedFile{{Path: "internal/foo/foo.go"}}},
		Judges: []JudgeVerdict{{
			Confidence: 0.92,
			Scores: map[string]float64{
				"correctness":     0.4,
				"maintainability": 0.4,
				"test_quality":    0.2,
				"performance":     0,
				"security":        0,
				"readability":     0.2,
				"observability":   0,
			},
		}},
		AcceptThreshold: 0.10,
		MinConfidence:   0.70,
	})
	if aggregate.Action != ActionAccept {
		t.Fatalf("expected accept, got %s", aggregate.Action)
	}
}
