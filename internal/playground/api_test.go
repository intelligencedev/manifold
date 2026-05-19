package playground

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"manifold/internal/playground/dataset"
	"manifold/internal/playground/eval"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/provider"
	"manifold/internal/playground/worker"
)

func TestStartRunValidatesUnknownSpecialist(t *testing.T) {
	t.Parallel()

	store := newFakePlaygroundStore()
	store.experiments["exp-1"] = experiment.ExperimentSpec{
		ID:        "exp-1",
		DatasetID: "dataset-1",
		Variants:  []experiment.Variant{{ID: "variant-1", PromptTemplate: "Hi"}},
		Execution: &experiment.ExecutionConfig{SpecialistName: "missing"},
	}
	service := NewService(
		Config{SpecialistValidator: fakeSpecialistValidator{err: ErrSpecialistNotFound}},
		nil,
		dataset.NewService(fakeDatasetStore{}),
		experiment.NewRepository(),
		experiment.NewPlanner(experiment.PlannerConfig{}),
		worker.NewWorker(fakePlaygroundProvider{}, nil),
		eval.NewRunner(eval.NewRegistry(), fakePlaygroundProvider{}),
		store,
	)

	_, err := service.StartRun(context.Background(), "exp-1")
	require.ErrorIs(t, err, ErrSpecialistNotFound)
}

func TestStartRunValidatesPausedSpecialist(t *testing.T) {
	t.Parallel()

	store := newFakePlaygroundStore()
	store.experiments["exp-1"] = experiment.ExperimentSpec{
		ID:        "exp-1",
		DatasetID: "dataset-1",
		Variants:  []experiment.Variant{{ID: "variant-1", PromptTemplate: "Hi"}},
		Execution: &experiment.ExecutionConfig{SpecialistName: "paused"},
	}
	service := NewService(
		Config{SpecialistValidator: fakeSpecialistValidator{err: ErrSpecialistPaused}},
		nil,
		dataset.NewService(fakeDatasetStore{}),
		experiment.NewRepository(),
		experiment.NewPlanner(experiment.PlannerConfig{}),
		worker.NewWorker(fakePlaygroundProvider{}, nil),
		eval.NewRunner(eval.NewRegistry(), fakePlaygroundProvider{}),
		store,
	)

	_, err := service.StartRun(context.Background(), "exp-1")
	require.ErrorIs(t, err, ErrSpecialistPaused)
}

func TestStartRunDirectProviderStillExecutes(t *testing.T) {
	t.Parallel()

	store := newFakePlaygroundStore()
	store.experiments["exp-1"] = experiment.ExperimentSpec{
		ID:        "exp-1",
		DatasetID: "dataset-1",
		Variants: []experiment.Variant{{
			ID:             "variant-1",
			Model:          "direct-model",
			PromptTemplate: "Hello {{name}}",
		}},
	}
	datasets := fakeDatasetStore{
		rows: []dataset.Row{{
			ID:     "row-1",
			Inputs: map[string]any{"name": "Ada"},
		}},
	}
	prov := fakePlaygroundProvider{}
	service := NewService(
		Config{},
		nil,
		dataset.NewService(datasets),
		experiment.NewRepository(),
		experiment.NewPlanner(experiment.PlannerConfig{}),
		worker.NewWorker(prov, nil),
		eval.NewRunner(eval.NewRegistry(), prov),
		store,
	)

	run, err := service.StartRun(context.Background(), "exp-1")
	require.NoError(t, err)
	require.Equal(t, RunStatusCompleted, run.Status)
	require.Len(t, store.results[run.ID], 1)
	result := store.results[run.ID][0]
	require.Equal(t, "Hello Ada -> direct-model", result.Output)
	require.Equal(t, "direct-model", result.Model)
	require.Equal(t, "fake", result.ProviderName)
	require.Nil(t, result.Execution)
}

type fakeSpecialistValidator struct {
	err error
}

func (v fakeSpecialistValidator) ValidateSpecialist(context.Context, int64, string) error {
	return v.err
}

type fakePlaygroundProvider struct{}

func (fakePlaygroundProvider) Name() string { return "fake" }

func (fakePlaygroundProvider) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	return provider.Response{
		Output:       req.Prompt + " -> " + req.Model,
		Tokens:       7,
		ProviderName: "fake",
		Model:        req.Model,
	}, nil
}

type fakeDatasetStore struct {
	rows []dataset.Row
}

func (s fakeDatasetStore) CreateDataset(context.Context, dataset.Dataset) (dataset.Dataset, error) {
	return dataset.Dataset{}, nil
}

func (s fakeDatasetStore) UpdateDataset(context.Context, dataset.Dataset) (dataset.Dataset, error) {
	return dataset.Dataset{}, nil
}

func (s fakeDatasetStore) GetDataset(context.Context, string) (dataset.Dataset, bool, error) {
	return dataset.Dataset{}, false, nil
}

func (s fakeDatasetStore) ListDatasets(context.Context) ([]dataset.Dataset, error) {
	return nil, nil
}

func (s fakeDatasetStore) CreateSnapshot(context.Context, dataset.Snapshot, []dataset.Row) (dataset.Snapshot, error) {
	return dataset.Snapshot{}, nil
}

func (s fakeDatasetStore) ListSnapshotRows(context.Context, string, string) ([]dataset.Row, error) {
	return append([]dataset.Row(nil), s.rows...), nil
}

func (s fakeDatasetStore) DeleteDataset(context.Context, string) error {
	return nil
}

type fakePlaygroundStore struct {
	experiments map[string]experiment.ExperimentSpec
	runs        map[string][]Run
	results     map[string][]RunResult
}

func newFakePlaygroundStore() *fakePlaygroundStore {
	return &fakePlaygroundStore{
		experiments: make(map[string]experiment.ExperimentSpec),
		runs:        make(map[string][]Run),
		results:     make(map[string][]RunResult),
	}
}

func (s *fakePlaygroundStore) CreateExperiment(_ context.Context, spec experiment.ExperimentSpec) (experiment.ExperimentSpec, error) {
	s.experiments[spec.ID] = spec
	return spec, nil
}

func (s *fakePlaygroundStore) GetExperiment(_ context.Context, id string) (experiment.ExperimentSpec, bool, error) {
	spec, ok := s.experiments[id]
	return spec, ok, nil
}

func (s *fakePlaygroundStore) ListExperiments(context.Context) ([]experiment.ExperimentSpec, error) {
	out := make([]experiment.ExperimentSpec, 0, len(s.experiments))
	for _, spec := range s.experiments {
		out = append(out, spec)
	}
	return out, nil
}

func (s *fakePlaygroundStore) CreateRun(_ context.Context, run Run) (Run, error) {
	s.runs[run.ExperimentID] = append(s.runs[run.ExperimentID], run)
	return run, nil
}

func (s *fakePlaygroundStore) UpdateRunStatus(_ context.Context, id string, status RunStatus, endedAt time.Time, errMsg string, metrics map[string]float64) error {
	for expID, runs := range s.runs {
		for i := range runs {
			if runs[i].ID != id {
				continue
			}
			runs[i].Status = status
			runs[i].EndedAt = endedAt
			runs[i].Error = errMsg
			runs[i].Metrics = metrics
			s.runs[expID] = runs
			return nil
		}
	}
	return nil
}

func (s *fakePlaygroundStore) AppendResults(_ context.Context, runID string, results []RunResult) error {
	s.results[runID] = append(s.results[runID], results...)
	return nil
}

func (s *fakePlaygroundStore) ListRuns(_ context.Context, experimentID string) ([]Run, error) {
	return append([]Run(nil), s.runs[experimentID]...), nil
}

func (s *fakePlaygroundStore) ListRunResults(_ context.Context, runID string) ([]RunResult, error) {
	return append([]RunResult(nil), s.results[runID]...), nil
}

func (s *fakePlaygroundStore) DeleteExperiment(_ context.Context, id string) error {
	delete(s.experiments, id)
	delete(s.runs, id)
	return nil
}
