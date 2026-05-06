package judge

import (
	"testing"

	"manifold/internal/codeqa"
)

func TestParseResponseSwapRemapsToAfterDelta(t *testing.T) {
	bundle := codeqa.DiffBundle{Files: []codeqa.ChangedFile{{Path: "foo.go"}}}
	verdict, err := ParseResponse("judge", `{"verdict":"option_b_better","confidence":0.8,"scores":{"correctness":0.4},"evidence":["foo.go"]}`, false, bundle, nil)
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if verdict.Verdict != "after_better" {
		t.Fatalf("expected after_better, got %s", verdict.Verdict)
	}
	if verdict.Scores["correctness"] != -0.4 {
		t.Fatalf("expected score remap to -0.4, got %v", verdict.Scores["correctness"])
	}
}

func TestParseResponseRepairsMalformedJSON(t *testing.T) {
	bundle := codeqa.DiffBundle{Files: []codeqa.ChangedFile{{Path: "foo.go"}}}
	verdict, err := ParseResponse("judge", "not json", true, bundle, func(string) (string, error) {
		return `{"verdict":"option_a_better","confidence":0.9,"scores":{"correctness":0.5},"evidence":["foo.go"]}`, nil
	})
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if verdict.Verdict != "after_better" {
		t.Fatalf("expected after_better after repair, got %s", verdict.Verdict)
	}
}

func TestParseResponsePenalizesHallucinatedEvidence(t *testing.T) {
	bundle := codeqa.DiffBundle{Files: []codeqa.ChangedFile{{Path: "foo.go"}}}
	verdict, err := ParseResponse("judge", `{"verdict":"tie","confidence":0.8,"scores":{},"evidence":["foo.go","ghost.go"]}`, true, bundle, nil)
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if verdict.Confidence != 0.75 {
		t.Fatalf("expected confidence penalty to 0.75, got %v", verdict.Confidence)
	}
	if len(verdict.Evidence) != 1 || verdict.Evidence[0] != "foo.go" {
		t.Fatalf("expected only valid evidence, got %v", verdict.Evidence)
	}
}
