package service

import (
	"fmt"
	"strings"

	"manifold/internal/codeqa"
)

func renderMarkdown(result codeqa.RunResult) string {
	var b strings.Builder
	b.WriteString("# Code QA Report\n\n")
	b.WriteString(fmt.Sprintf("- Run ID: %s\n", result.RunID))
	b.WriteString(fmt.Sprintf("- Repository: %s\n", result.Repository))
	b.WriteString(fmt.Sprintf("- Mode: %s\n", result.Mode))
	b.WriteString(fmt.Sprintf("- Base: %s\n", result.Diff.BaseRef))
	b.WriteString(fmt.Sprintf("- Head: %s\n", result.Diff.HeadRef))
	b.WriteString(fmt.Sprintf("- Action: %s\n", result.Aggregate.Action))
	b.WriteString(fmt.Sprintf("- Quality Delta: %.2f\n", result.Aggregate.QualityDelta))
	b.WriteString(fmt.Sprintf("- Confidence: %.2f\n\n", result.Aggregate.Confidence))
	b.WriteString("## Gates\n\n")
	for _, gate := range result.Gates {
		b.WriteString(fmt.Sprintf("- %s [%s]: ok=%t hard_fail=%t\n", gate.Name, gate.Ref, gate.OK, gate.HardFail))
	}
	b.WriteString("\n## Judges\n\n")
	for _, judge := range result.Judges {
		b.WriteString(fmt.Sprintf("- %s: verdict=%s confidence=%.2f\n", judge.JudgeID, judge.Verdict, judge.Confidence))
	}
	b.WriteString("\n## Rationale\n\n")
	b.WriteString(result.Aggregate.Rationale)
	b.WriteString("\n")
	if len(result.Artifacts) > 0 {
		b.WriteString("\n## Artifacts\n\n")
		for name, path := range result.Artifacts {
			b.WriteString(fmt.Sprintf("- %s: %s\n", name, path))
		}
	}
	return b.String()
}
