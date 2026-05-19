package experiment

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/playground/dataset"

	"github.com/stretchr/testify/require"
)

func TestPlannerChunking(t *testing.T) {
	t.Parallel()

	rows := make([]dataset.Row, 10)
	for i := range rows {
		rows[i] = dataset.Row{ID: string(rune('a' + i))}
	}

	spec := ExperimentSpec{
		Variants:    []Variant{{ID: "v1"}, {ID: "v2"}},
		Concurrency: ConcurrencyConfig{MaxRowsPerShard: 3, MaxVariantsPerRun: 1},
	}

	planner := NewPlanner(PlannerConfig{MaxRowsPerShard: 4, MaxVariantsPerShard: 2})
	plan, err := planner.Plan(context.Background(), spec, rows)
	require.NoError(t, err)
	require.Len(t, plan.Shards, 4)
	for _, shard := range plan.Shards {
		require.LessOrEqual(t, len(shard.Rows), 3)
		require.Len(t, shard.Variants, 1)
	}
}

func TestExperimentSpecExecutionRoundTrip(t *testing.T) {
	t.Parallel()

	spec := ExperimentSpec{
		ID:        "exp-1",
		Name:      "Tool run",
		DatasetID: "dataset-1",
		Variants:  []Variant{{ID: "variant-1", PromptVersionID: "prompt-1"}},
		Execution: &ExecutionConfig{
			SpecialistName: "researcher",
		},
	}

	data, err := json.Marshal(spec)
	require.NoError(t, err)

	var decoded ExperimentSpec
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.Execution)
	require.Equal(t, "researcher", decoded.Execution.SpecialistName)
}

func TestExecutionConfigNormalizeAndClone(t *testing.T) {
	t.Parallel()

	require.Nil(t, NormalizeExecution(nil))
	require.Nil(t, NormalizeExecution(&ExecutionConfig{SpecialistName: "  "}))

	normalized := NormalizeExecution(&ExecutionConfig{SpecialistName: "  researcher  "})
	require.NotNil(t, normalized)
	require.Equal(t, "researcher", normalized.SpecialistName)

	clone := CloneExecution(normalized)
	require.NotSame(t, normalized, clone)
	require.Equal(t, normalized, clone)
}

func TestPlannerRequiresVariant(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(PlannerConfig{})
	_, err := planner.Plan(context.Background(), ExperimentSpec{}, []dataset.Row{{ID: "1"}})
	require.Error(t, err)
}
