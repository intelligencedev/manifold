package playground

import (
	"context"
	"errors"
	"fmt"
	"time"

	"intelligence.dev/internal/playground/artifacts"
	"intelligence.dev/internal/playground/dataset"
	"intelligence.dev/internal/playground/eval"
	"intelligence.dev/internal/playground/experiment"
	"intelligence.dev/internal/playground/provider"
	"intelligence.dev/internal/playground/registry"
	"intelligence.dev/internal/playground/worker"
)

var (
	// ErrActiveRun indicates the service refuses to start another run while one is active for the same experiment.
	ErrActiveRun = errors.New("playground: experiment already has an active run")
	// ErrUnknownExperiment is returned when attempting to interact with an experiment that has not been registered.
	ErrUnknownExperiment = errors.New("playground: unknown experiment")
)

// Service wires together the playground components and provides a cohesive
// interface for orchestration layers and HTTP handlers. The service is
// intentionally thin; business logic lives inside the dedicated packages.
type Service struct {
	cfg         Config
	registry    *registry.Registry
	datasets    *dataset.Service
	experiments *experiment.Repository
	planner     *experiment.Planner
	workers     worker.Executor
	evals       *eval.Runner
	store       RunStore
}

// RunStore captures the persistence requirements the service expects.
type RunStore interface {
	CreateExperiment(ctx context.Context, spec experiment.ExperimentSpec) (experiment.ExperimentSpec, error)
	GetExperiment(ctx context.Context, id string) (experiment.ExperimentSpec, bool, error)
	ListExperiments(ctx context.Context) ([]experiment.ExperimentSpec, error)
	CreateRun(ctx context.Context, run Run) (Run, error)
	UpdateRunStatus(ctx context.Context, id string, status RunStatus, endedAt time.Time, errMsg string, metrics map[string]float64) error
	AppendResults(ctx context.Context, runID string, results []RunResult) error
	ListRuns(ctx context.Context, experimentID string) ([]Run, error)
	ListRunResults(ctx context.Context, runID string) ([]RunResult, error)
}

// Config tunes runtime aspects of the service.
type Config struct {
	MaxConcurrentShards int
}

// NewService assembles the playground service.
func NewService(cfg Config, reg *registry.Registry, datasets *dataset.Service, repo *experiment.Repository, planner *experiment.Planner, workers worker.Executor, evals *eval.Runner, store RunStore) *Service {
	return &Service{
		cfg:         cfg,
		registry:    reg,
		datasets:    datasets,
		experiments: repo,
		planner:     planner,
		workers:     workers,
		evals:       evals,
		store:       store,
	}
}

// ListPrompts proxies prompt listing to the registry to keep handlers lean.
func (s *Service) ListPrompts(ctx context.Context, filter registry.ListFilter) ([]registry.Prompt, error) {
	return s.registry.ListPrompts(ctx, filter)
}

// CreatePrompt registers a new prompt in the registry and persists it.
func (s *Service) CreatePrompt(ctx context.Context, prompt registry.Prompt) (registry.Prompt, error) {
	return s.registry.CreatePrompt(ctx, prompt)
}

// CreatePromptVersion stores a new version for a prompt and returns the persisted value.
func (s *Service) CreatePromptVersion(ctx context.Context, promptID string, version registry.PromptVersion) (registry.PromptVersion, error) {
	return s.registry.CreatePromptVersion(ctx, promptID, version)
}

// GetPrompt fetches a prompt by ID.
func (s *Service) GetPrompt(ctx context.Context, id string) (registry.Prompt, bool, error) {
	return s.registry.GetPrompt(ctx, id)
}

// GetPromptVersion fetches a prompt version by ID.
func (s *Service) GetPromptVersion(ctx context.Context, id string) (registry.PromptVersion, bool, error) {
	return s.registry.GetPromptVersion(ctx, id)
}

// ListPromptVersions lists prompt versions.
func (s *Service) ListPromptVersions(ctx context.Context, promptID string) ([]registry.PromptVersion, error) {
	return s.registry.ListPromptVersions(ctx, promptID)
}

// RegisterDataset writes the dataset metadata and rows.
func (s *Service) RegisterDataset(ctx context.Context, ds dataset.Dataset, rows []dataset.Row) (dataset.Dataset, error) {
	return s.datasets.CreateDataset(ctx, ds, rows)
}

// UpdateDataset updates dataset metadata and rows.
func (s *Service) UpdateDataset(ctx context.Context, ds dataset.Dataset, rows []dataset.Row) (dataset.Dataset, error) {
	return s.datasets.UpdateDataset(ctx, ds, rows)
}

// ListDatasets returns dataset metadata for UI listing.
func (s *Service) ListDatasets(ctx context.Context) ([]dataset.Dataset, error) {
	return s.datasets.ListDatasets(ctx)
}

// GetDataset fetches a dataset and indicates whether it exists.
func (s *Service) GetDataset(ctx context.Context, id string) (dataset.Dataset, bool, error) {
	return s.datasets.GetDataset(ctx, id)
}

// ListDatasetRows returns the current rows for the dataset's initial snapshot.
func (s *Service) ListDatasetRows(ctx context.Context, id string) ([]dataset.Row, error) {
	return s.datasets.ResolveSnapshotRows(ctx, id, "", "")
}

// CreateExperiment registers an experiment specification and persists it.
func (s *Service) CreateExperiment(ctx context.Context, spec experiment.ExperimentSpec) (experiment.ExperimentSpec, error) {
	saved, err := s.store.CreateExperiment(ctx, spec)
	if err != nil {
		return experiment.ExperimentSpec{}, err
	}
	s.experiments.Save(saved)
	return saved, nil
}

// ListExperiments returns all experiment specifications persisted in the store.
func (s *Service) ListExperiments(ctx context.Context) ([]experiment.ExperimentSpec, error) {
	return s.store.ListExperiments(ctx)
}

// GetExperiment fetches a stored experiment specification.
func (s *Service) GetExperiment(ctx context.Context, id string) (experiment.ExperimentSpec, bool, error) {
	if spec, ok := s.experiments.Get(id); ok {
		return spec, true, nil
	}
	spec, ok, err := s.store.GetExperiment(ctx, id)
	if err != nil {
		return experiment.ExperimentSpec{}, false, err
	}
	if ok {
		s.experiments.Save(spec)
	}
	return spec, ok, nil
}

// StartRun plans and executes a run for an experiment. The execution happens
// synchronously for now; orchestration integration can expand this later.
func (s *Service) StartRun(ctx context.Context, experimentID string) (Run, error) {
	spec, ok, err := s.store.GetExperiment(ctx, experimentID)
	if err != nil {
		return Run{}, err
	}
	if !ok {
		return Run{}, ErrUnknownExperiment
	}
	s.experiments.Save(spec)

	runs, err := s.store.ListRuns(ctx, experimentID)
	if err != nil {
		return Run{}, err
	}
	for _, r := range runs {
		if r.Status == RunStatusRunning || r.Status == RunStatusPending {
			return Run{}, ErrActiveRun
		}
	}

	runID := worker.NewRunID()
	plan, enrichedSpec, err := s.planRun(ctx, spec)
	if err != nil {
		return Run{}, err
	}
	spec = enrichedSpec

	run := Run{
		ID:           runID,
		ExperimentID: experimentID,
		Plan:         plan,
		Status:       RunStatusPending,
		CreatedAt:    time.Now().UTC(),
	}

	run, err = s.store.CreateRun(ctx, run)
	if err != nil {
		return Run{}, err
	}

	// Execute synchronously shard by shard.
	run.Status = RunStatusRunning
	run.StartedAt = time.Now().UTC()
	if err := s.store.UpdateRunStatus(ctx, run.ID, RunStatusRunning, time.Time{}, "", nil); err != nil {
		return Run{}, err
	}

	var workerResults []worker.Result
	for _, shard := range run.Plan.Shards {
		if err := ctx.Err(); err != nil {
			return s.failRun(ctx, run, err)
		}
		tasks := worker.TasksFromShard(run.ID, spec, shard)
		for _, task := range tasks {
			res, execErr := s.workers.ExecuteTask(ctx, task)
			if execErr != nil {
				return s.failRun(ctx, run, execErr)
			}
			workerResults = append(workerResults, res)
		}
	}

	metrics, updatedResults, err := s.evals.Evaluate(ctx, spec, workerResults)
	if err != nil {
		return s.failRun(ctx, run, fmt.Errorf("evaluate run: %w", err))
	}
	results := make([]RunResult, 0, len(updatedResults))
	for _, res := range updatedResults {
		results = append(results, RunResultFromWorker(res))
	}

	run.Status = RunStatusCompleted
	run.EndedAt = time.Now().UTC()
	run.Metrics = metrics
	if err := s.store.AppendResults(ctx, run.ID, results); err != nil {
		return Run{}, err
	}
	if err := s.store.UpdateRunStatus(ctx, run.ID, run.Status, run.EndedAt, "", run.Metrics); err != nil {
		return Run{}, err
	}
	return run, nil
}

// ListRuns returns existing runs for an experiment.
func (s *Service) ListRuns(ctx context.Context, experimentID string) ([]Run, error) {
	return s.store.ListRuns(ctx, experimentID)
}

func (s *Service) planRun(ctx context.Context, spec experiment.ExperimentSpec) (experiment.RunPlan, experiment.ExperimentSpec, error) {
	enriched, err := s.enrichVariants(ctx, spec)
	if err != nil {
		return experiment.RunPlan{}, experiment.ExperimentSpec{}, err
	}
	rows, err := s.datasets.ResolveSnapshotRows(ctx, enriched.DatasetID, enriched.SnapshotID, enriched.SliceExpr)
	if err != nil {
		return experiment.RunPlan{}, experiment.ExperimentSpec{}, err
	}
	plan, err := s.planner.Plan(ctx, enriched, rows)
	if err != nil {
		return experiment.RunPlan{}, experiment.ExperimentSpec{}, err
	}
	return plan, enriched, nil
}

func (s *Service) enrichVariants(ctx context.Context, spec experiment.ExperimentSpec) (experiment.ExperimentSpec, error) {
	enriched := spec
	enriched.Variants = make([]experiment.Variant, len(spec.Variants))
	copy(enriched.Variants, spec.Variants)
	for i, variant := range enriched.Variants {
		if variant.PromptTemplate != "" {
			continue
		}
		version, ok, err := s.registry.GetPromptVersion(ctx, variant.PromptVersionID)
		if err != nil {
			return experiment.ExperimentSpec{}, fmt.Errorf("load prompt version %s: %w", variant.PromptVersionID, err)
		}
		if !ok {
			return experiment.ExperimentSpec{}, fmt.Errorf("prompt version %s not found", variant.PromptVersionID)
		}
		variant.PromptTemplate = version.Template
		variant.Variables = version.Variables
		enriched.Variants[i] = variant
	}
	return enriched, nil
}

func (s *Service) failRun(ctx context.Context, run Run, err error) (Run, error) {
	run.Status = RunStatusFailed
	run.EndedAt = time.Now().UTC()
	run.Error = err.Error()
	_ = s.store.UpdateRunStatus(ctx, run.ID, run.Status, run.EndedAt, run.Error, nil)
	return run, err
}

// ListRunResults returns row-level outputs for a run.
func (s *Service) ListRunResults(ctx context.Context, runID string) ([]RunResult, error) {
	return s.store.ListRunResults(ctx, runID)
}

// RunResultFromWorker adapts worker results into the public RunResult type.
func RunResultFromWorker(res worker.Result) RunResult {
	return RunResult{
		ID:              res.ID,
		RunID:           res.RunID,
		RowID:           res.RowID,
		VariantID:       res.VariantID,
		PromptVersionID: res.PromptVersionID,
		Model:           res.Model,
		Rendered:        res.RenderedPrompt,
		Output:          res.Output,
		Tokens:          res.Tokens,
		Latency:         res.Latency,
		ProviderName:    res.ProviderName,
		Artifacts:       cloneStringMap(res.Artifacts),
		Scores:          cloneScores(res.Scores),
		Expected:        res.Expected,
	}
}

func cloneScores(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// NewMockService wires an in-memory instance useful for unit tests and fast prototyping.
func NewMockService(provider provider.Provider) *Service {
	artifactStore := artifacts.NewInMemoryStore()
	datasetStore := dataset.NewInMemoryStore()
	datasetSvc := dataset.NewService(datasetStore)
	reg := registry.New(registry.NewInMemoryStore())
	repo := experiment.NewRepository()
	planner := experiment.NewPlanner(experiment.PlannerConfig{MaxRowsPerShard: 32, MaxVariantsPerShard: 4})
	exec := worker.NewWorker(provider, artifactStore)
	evals := eval.NewRunner(eval.NewRegistry(), provider)
	runStore := NewInMemoryRunStore()
	return NewService(Config{MaxConcurrentShards: 1}, reg, datasetSvc, repo, planner, exec, evals, runStore)
}
