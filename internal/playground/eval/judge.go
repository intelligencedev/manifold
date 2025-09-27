package eval

import (
	"context"
	"fmt"
	"strings"

	"intelligence.dev/internal/playground/experiment"
	"intelligence.dev/internal/playground/provider"
	"intelligence.dev/internal/playground/worker"
)

const judgeMetric = "judge/accuracy"

// judgeEvaluator uses a provider to grade outputs when available, otherwise falls back to expectation matching.
type judgeEvaluator struct {
	provider provider.Provider
}

func newJudgeEvaluator(_ experiment.EvaluatorConfig, prov provider.Provider) (Evaluator, error) {
	return &judgeEvaluator{provider: prov}, nil
}

func (j *judgeEvaluator) Name() string { return "llm-judge" }

func (j *judgeEvaluator) Evaluate(ctx context.Context, _ experiment.ExperimentSpec, results []worker.Result) (Outcome, error) {
	if len(results) == 0 {
		return Outcome{Aggregate: map[string]float64{judgeMetric: 0}, Scores: map[int]map[string]float64{}}, nil
	}
	scores := make(map[int]map[string]float64)
	total := 0.0
	for idx, res := range results {
		select {
		case <-ctx.Done():
			return Outcome{}, ctx.Err()
		default:
		}
		score := j.scoreResult(ctx, res)
		scores[idx] = map[string]float64{judgeMetric: score}
		total += score
	}
	return Outcome{
		Aggregate: map[string]float64{judgeMetric: total / float64(len(results))},
		Scores:    scores,
	}, nil
}

func (j *judgeEvaluator) scoreResult(ctx context.Context, res worker.Result) float64 {
	if res.Expected == nil {
		return 0.5
	}
	expectedStr := strings.TrimSpace(fmt.Sprint(res.Expected))
	output := strings.TrimSpace(res.Output)
	if strings.EqualFold(expectedStr, output) {
		return 1
	}
	if j.provider == nil {
		return 0
	}
	_, err := j.provider.Complete(ctx, provider.Request{
		Model:  res.Model,
		Prompt: fmt.Sprintf("Judge correctness. expected=%q output=%q", expectedStr, output),
	})
	if err != nil {
		return 0
	}
	return 0.5
}
