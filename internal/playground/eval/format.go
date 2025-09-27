package eval

import (
	"context"
	"strings"

	"intelligence.dev/internal/playground/experiment"
	"intelligence.dev/internal/playground/provider"
	"intelligence.dev/internal/playground/worker"
)

const formatMetric = "format/pass_rate"

// formatEvaluator ensures outputs respect simple formatting expectations.
type formatEvaluator struct {
	pattern string
}

func newFormatEvaluator(cfg experiment.EvaluatorConfig, _ provider.Provider) (Evaluator, error) {
	pattern := ""
	if cfg.Params != nil {
		if v, ok := cfg.Params["pattern"].(string); ok {
			pattern = v
		}
	}
	return &formatEvaluator{pattern: pattern}, nil
}

func (f *formatEvaluator) Name() string { return "format" }

func (f *formatEvaluator) Evaluate(ctx context.Context, _ experiment.ExperimentSpec, results []worker.Result) (Outcome, error) {
	passCount := 0.0
	scores := make(map[int]map[string]float64)
	for idx, r := range results {
		select {
		case <-ctx.Done():
			return Outcome{}, ctx.Err()
		default:
		}
		pass := false
		trimmed := strings.TrimSpace(r.Output)
		if f.pattern == "" {
			pass = trimmed != ""
		} else {
			pass = strings.Contains(trimmed, f.pattern)
		}
		if pass {
			passCount++
		}
		scores[idx] = map[string]float64{formatMetric: boolScore(pass)}
	}

	total := float64(len(results))
	if total == 0 {
		total = 1
	}
	return Outcome{
		Aggregate: map[string]float64{formatMetric: passCount / total},
		Scores:    scores,
	}, nil
}

func boolScore(pass bool) float64 {
	if pass {
		return 1
	}
	return 0
}
