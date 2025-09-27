package eval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"intelligence.dev/internal/playground/experiment"
	"intelligence.dev/internal/playground/provider"
	"intelligence.dev/internal/playground/worker"
)

func TestRunnerEvaluateAggregatesMetrics(t *testing.T) {
	t.Parallel()

	spec := experiment.ExperimentSpec{
		Evaluators: []experiment.EvaluatorConfig{
			{Name: "format"},
			{Name: "llm-judge"},
		},
	}

	results := []worker.Result{
		{Output: "hello", Expected: "hello"},
		{Output: "", Expected: "world"},
	}

	runner := NewRunner(NewRegistry(), provider.NewMockProvider("mock"))

	metrics, updated, err := runner.Evaluate(context.Background(), spec, results)
	require.NoError(t, err)
	require.Len(t, updated, len(results))
	require.Contains(t, metrics, formatMetric)
	require.Contains(t, metrics, judgeMetric)
	for _, r := range updated {
		require.NotNil(t, r.Scores)
	}
}
