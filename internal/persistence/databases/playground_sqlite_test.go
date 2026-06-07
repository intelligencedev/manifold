package databases

import (
	"context"
	"testing"
	"time"

	"manifold/internal/playground"
	"manifold/internal/playground/dataset"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/registry"
)

func TestSQLitePlaygroundStoreParity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := NewSQLitePlaygroundStore(ctx, openTestSQLite(t))
	if err != nil {
		t.Fatalf("NewSQLitePlaygroundStore: %v", err)
	}

	now := time.Now().UTC()
	prompt := registry.Prompt{
		ID:        "prompt-1",
		Name:      "SQLite prompt",
		Tags:      []string{"sqlite"},
		CreatedAt: now,
	}
	if _, err := store.CreatePrompt(ctx, prompt); err != nil {
		t.Fatalf("CreatePrompt: %v", err)
	}
	if _, err := store.CreatePromptVersion(ctx, registry.PromptVersion{
		ID:        "version-1",
		PromptID:  prompt.ID,
		Semver:    "1.0.0",
		Template:  "Hello {{name}}",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreatePromptVersion: %v", err)
	}
	prompts, err := store.ListPrompts(ctx, registry.ListFilter{Tag: "sqlite"})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) != 1 || prompts[0].ID != prompt.ID {
		t.Fatalf("unexpected prompts: %+v", prompts)
	}

	ds := dataset.Dataset{ID: "dataset-1", Name: "Dataset", CreatedAt: now}
	if _, err := store.CreateDataset(ctx, ds); err != nil {
		t.Fatalf("CreateDataset: %v", err)
	}
	snapshot := dataset.Snapshot{ID: "dataset-1-initial", DatasetID: ds.ID, CreatedAt: now}
	rows := []dataset.Row{{ID: "row-1", Inputs: map[string]any{"name": "Ada"}, Split: "test"}}
	if _, err := store.CreateSnapshot(ctx, snapshot, rows); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	gotRows, err := store.ListSnapshotRows(ctx, ds.ID, snapshot.ID)
	if err != nil {
		t.Fatalf("ListSnapshotRows: %v", err)
	}
	if len(gotRows) != 1 || gotRows[0].ID != "row-1" {
		t.Fatalf("unexpected rows: %+v", gotRows)
	}

	spec := experiment.ExperimentSpec{
		ID:        "experiment-1",
		Name:      "Experiment",
		DatasetID: ds.ID,
		Variants:  []experiment.Variant{{ID: "variant-1", PromptTemplate: "Hello {{name}}"}},
		CreatedAt: now,
	}
	if _, err := store.CreateExperiment(ctx, spec); err != nil {
		t.Fatalf("CreateExperiment: %v", err)
	}
	run := playground.Run{
		ID:           "run-1",
		ExperimentID: spec.ID,
		Status:       playground.RunStatusRunning,
		CreatedAt:    now,
		StartedAt:    now,
	}
	if _, err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := store.AppendResults(ctx, run.ID, []playground.RunResult{{
		ID:        "result-1",
		RunID:     run.ID,
		RowID:     "row-1",
		VariantID: "variant-1",
		Output:    "Hello Ada",
	}}); err != nil {
		t.Fatalf("AppendResults: %v", err)
	}
	if err := store.UpdateRunStatus(ctx, run.ID, playground.RunStatusCompleted, now.Add(time.Second), "", map[string]float64{"score": 1}); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}
	runs, err := store.ListRuns(ctx, spec.ID)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != playground.RunStatusCompleted || runs[0].Metrics["score"] != 1 {
		t.Fatalf("unexpected runs: %+v", runs)
	}
	results, err := store.ListRunResults(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListRunResults: %v", err)
	}
	if len(results) != 1 || results[0].ID != "result-1" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if err := store.DeleteExperiment(ctx, spec.ID); err != nil {
		t.Fatalf("DeleteExperiment: %v", err)
	}
	runs, err = store.ListRuns(ctx, spec.ID)
	if err != nil {
		t.Fatalf("ListRuns after delete: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected deleted runs, got %+v", runs)
	}
}
