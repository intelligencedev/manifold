package codeqa

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var DefaultScoreWeights = map[string]float64{
	"correctness":     0.30,
	"maintainability": 0.20,
	"test_quality":    0.15,
	"performance":     0.10,
	"security":        0.10,
	"readability":     0.10,
	"observability":   0.05,
}

type DecisionInput struct {
	Mode            RunMode
	Bundle          DiffBundle
	Gates           []GateResult
	Judges          []JudgeVerdict
	AcceptThreshold float64
	MinConfidence   float64
	HighRiskGlobs   []string
}

func Decide(in DecisionInput) Aggregate {
	qualityDelta := weightedJudgeDelta(in.Judges)
	confidence := judgeConfidence(in.Judges)
	hardFailures := collectHardFailures(in.Gates)
	blocking := collectBlockingConcerns(in.Judges)
	highRisk := touchesHighRisk(in.Bundle.Files, in.HighRiskGlobs)
	disagreement := judgeDisagreement(in.Judges)

	if len(hardFailures) > 0 {
		action := ActionReject
		if in.Mode == ModeOptimize {
			action = ActionRevertCandidate
		}
		return decisionAggregate(qualityDelta, confidence, action, "deterministic gates failed", hardFailures)
	}

	if in.Bundle.Truncated {
		return decisionAggregate(qualityDelta, confidence, ActionHumanReview, "diff bundle was truncated", nil)
	}

	if highRisk {
		return decisionAggregate(qualityDelta, confidence, ActionHumanReview, "high-risk paths require human review", nil)
	}

	if len(in.Judges) == 0 {
		return decisionAggregate(qualityDelta, confidence, ActionHumanReview, "missing judge evidence", nil)
	}

	if confidence < in.MinConfidence {
		return decisionAggregate(qualityDelta, confidence, ActionHumanReview, "judge confidence below policy threshold", nil)
	}

	if disagreement > 0.40 {
		return decisionAggregate(qualityDelta, confidence, ActionHumanReview, "judge disagreement exceeded policy threshold", nil)
	}

	if len(blocking) > 0 {
		return decisionAggregate(qualityDelta, confidence, ActionHumanReview, "judge reported blocking concerns", nil)
	}

	if qualityDelta >= in.AcceptThreshold {
		return decisionAggregate(qualityDelta, confidence, ActionAccept, fmt.Sprintf("quality delta %.2f met acceptance threshold", qualityDelta), nil)
	}

	return decisionAggregate(qualityDelta, confidence, ActionReject, "quality delta did not meet acceptance threshold", nil)
}

func decisionAggregate(qualityDelta, confidence float64, action RecommendedAction, rationale string, hardFailures []string) Aggregate {
	return Aggregate{
		QualityDelta: qualityDelta,
		Confidence:   confidence,
		HardFailures: hardFailures,
		Action:       action,
		Rationale:    rationale,
	}
}

func weightedJudgeDelta(judges []JudgeVerdict) float64 {
	if len(judges) == 0 {
		return 0
	}
	var total float64
	for _, judge := range judges {
		var score float64
		for dimension, weight := range DefaultScoreWeights {
			score += judge.Scores[dimension] * weight
		}
		total += score
	}
	return total / float64(len(judges))
}

func judgeConfidence(judges []JudgeVerdict) float64 {
	if len(judges) == 0 {
		return 0
	}
	var total float64
	for _, judge := range judges {
		total += judge.Confidence
	}
	return total / float64(len(judges))
}

func judgeDisagreement(judges []JudgeVerdict) float64 {
	if len(judges) < 2 {
		return 0
	}
	values := make([]float64, 0, len(judges))
	for _, judge := range judges {
		values = append(values, weightedJudgeDelta([]JudgeVerdict{judge}))
	}
	mean := 0.0
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}

func collectHardFailures(gates []GateResult) []string {
	out := make([]string, 0)
	for _, gate := range gates {
		if gate.HardFail {
			out = append(out, gate.Name+"@"+gate.Ref)
		}
	}
	sort.Strings(out)
	return out
}

func collectBlockingConcerns(judges []JudgeVerdict) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, judge := range judges {
		for _, concern := range judge.BlockingConcerns {
			concern = strings.TrimSpace(concern)
			if concern == "" {
				continue
			}
			if _, ok := seen[concern]; ok {
				continue
			}
			seen[concern] = struct{}{}
			out = append(out, concern)
		}
	}
	sort.Strings(out)
	return out
}

func touchesHighRisk(files []ChangedFile, patterns []string) bool {
	for _, file := range files {
		if MatchAny(file.Path, patterns) {
			return true
		}
	}
	return false
}
