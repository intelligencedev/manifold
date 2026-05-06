package judge

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"manifold/internal/codeqa"
)

type RepairFunc func(string) (string, error)

type rawVerdict struct {
	Verdict          string             `json:"verdict"`
	Confidence       float64            `json:"confidence"`
	Scores           map[string]float64 `json:"scores"`
	BlockingConcerns []string           `json:"blocking_concerns"`
	Evidence         []string           `json:"evidence"`
}

func ParseResponse(judgeID string, raw string, swap bool, bundle codeqa.DiffBundle, repair RepairFunc) (codeqa.JudgeVerdict, error) {
	parsed, err := parseRaw(raw)
	if err != nil && repair != nil {
		repaired, repairErr := repair(raw)
		if repairErr == nil {
			parsed, err = parseRaw(repaired)
		}
	}
	if err != nil {
		return codeqa.JudgeVerdict{}, err
	}
	verdict, scores := remapVerdict(parsed, swap)
	confidence := math.Max(0, math.Min(1, parsed.Confidence))
	validFiles := make(map[string]struct{}, len(bundle.Files))
	for _, file := range bundle.Files {
		validFiles[file.Path] = struct{}{}
	}
	validEvidence := make([]string, 0, len(parsed.Evidence))
	invalidEvidence := 0
	for _, evidence := range parsed.Evidence {
		evidence = strings.TrimSpace(evidence)
		if evidence == "" {
			continue
		}
		if _, ok := validFiles[evidence]; ok {
			validEvidence = append(validEvidence, evidence)
			continue
		}
		invalidEvidence++
	}
	confidence -= float64(invalidEvidence) * 0.05
	if confidence < 0 {
		confidence = 0
	}
	return codeqa.JudgeVerdict{
		JudgeID:          judgeID,
		Verdict:          verdict,
		Confidence:       confidence,
		Scores:           scores,
		BlockingConcerns: parsed.BlockingConcerns,
		SwapApplied:      swap,
		Evidence:         validEvidence,
	}, nil
}

func parseRaw(raw string) (rawVerdict, error) {
	jsonText := extractJSONObject(raw)
	if jsonText == "" {
		return rawVerdict{}, errors.New("judge response did not contain JSON object")
	}
	var parsed rawVerdict
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return rawVerdict{}, fmt.Errorf("unmarshal judge response: %w", err)
	}
	if parsed.Verdict == "" {
		return rawVerdict{}, errors.New("judge response missing verdict")
	}
	if parsed.Scores == nil {
		parsed.Scores = map[string]float64{}
	}
	for _, dimension := range Dimensions {
		score := parsed.Scores[dimension]
		if score > 1 {
			score = 1
		}
		if score < -1 {
			score = -1
		}
		parsed.Scores[dimension] = score
	}
	return parsed, nil
}

func extractJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return raw[start : end+1]
}

func remapVerdict(parsed rawVerdict, swap bool) (string, map[string]float64) {
	scores := make(map[string]float64, len(parsed.Scores))
	for dimension, score := range parsed.Scores {
		scores[dimension] = score
	}
	if !swap {
		for dimension, score := range scores {
			scores[dimension] = -score
		}
	}
	switch parsed.Verdict {
	case "option_a_better":
		if swap {
			return "after_better", scores
		}
		return "before_better", scores
	case "option_b_better":
		if swap {
			return "before_better", scores
		}
		return "after_better", scores
	case "tie":
		return "tie", scores
	default:
		return "insufficient", scores
	}
}
