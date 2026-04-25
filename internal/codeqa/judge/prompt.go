package judge

import (
	"fmt"
	"hash/fnv"
	"strings"

	"manifold/internal/codeqa"
)

type JudgeProfile struct {
	ID    string
	Focus string
}

func DefaultProfiles() []JudgeProfile {
	return []JudgeProfile{
		{ID: "judge-maintainability", Focus: "Focus on maintainability, cohesion, naming, and abstraction boundaries."},
		{ID: "judge-correctness-risk", Focus: "Focus on hidden behavior changes, regression risk, and semantic correctness."},
		{ID: "judge-tests", Focus: "Focus on test coverage quality, related tests, and whether the change is meaningfully validated."},
		{ID: "judge-security", Focus: "Focus on trust boundaries, injection risk, sensitive data handling, and unsafe filesystem or network behavior."},
	}
}

func SwapApplied(bundle codeqa.DiffBundle) bool {
	return SwapAppliedForJudge(bundle, "")
}

func SwapAppliedForJudge(bundle codeqa.DiffBundle, judgeID string) bool {
	h := fnv.New32a()
	_, _ = h.Write([]byte(bundle.BaseRef))
	_, _ = h.Write([]byte(bundle.HeadRef))
	_, _ = h.Write([]byte(bundle.UnifiedDiff))
	_, _ = h.Write([]byte(judgeID))
	return h.Sum32()%2 == 0
}

func BuildPrompt(profile JudgeProfile, bundle codeqa.DiffBundle, gates []codeqa.GateResult, swap bool) string {
	aLabel := "BASELINE"
	bLabel := "CANDIDATE"
	if swap {
		aLabel = "CANDIDATE"
		bLabel = "BASELINE"
	}
	filePaths := make([]string, 0, len(bundle.Files))
	for _, file := range bundle.Files {
		filePaths = append(filePaths, file.Path)
	}
	return fmt.Sprintf(`You are judging code quality between two options.

Return strict JSON only with this schema:
{
  "verdict": "option_a_better|option_b_better|tie|insufficient",
  "confidence": 0.0,
  "scores": {
    "correctness": 0.0,
    "maintainability": 0.0,
    "test_quality": 0.0,
    "performance": 0.0,
    "security": 0.0,
    "readability": 0.0,
    "observability": 0.0
  },
  "blocking_concerns": ["..."],
  "evidence": ["path/to/file.go"]
}

Rules:
- scores are in [-1,1] and positive means OPTION_A is better than OPTION_B.
- cite only files from this set: %s
- deterministic evidence dominates stylistic preference.
- if evidence is insufficient, use verdict "insufficient".
- %s

OPTION_A = %s
OPTION_B = %s

Gate summary:
%s

Repo context:
%s

Unified diff:
%s
`, strings.Join(filePaths, ", "), profile.Focus, aLabel, bLabel, formatGateSummary(gates), bundle.RepoContext, bundle.UnifiedDiff)
}

func BuildRepairPrompt(raw string) string {
	return "Repair the following response into strict JSON only. Do not add commentary.\n\n" + raw
}

func formatGateSummary(gates []codeqa.GateResult) string {
	parts := make([]string, 0, len(gates))
	for _, gate := range gates {
		parts = append(parts, fmt.Sprintf("- %s[%s]: ok=%t hard_fail=%t", gate.Name, gate.Ref, gate.OK, gate.HardFail))
	}
	return strings.Join(parts, "\n")
}
