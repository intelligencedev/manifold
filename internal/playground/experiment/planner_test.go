package experiment

import (
	"context"
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

func TestPlannerRequiresVariant(t *testing.T) {
	t.Parallel()

	planner := NewPlanner(PlannerConfig{})
	_, err := planner.Plan(context.Background(), ExperimentSpec{}, []dataset.Row{{ID: "1"}})
	require.Error(t, err)
}
