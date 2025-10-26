package playground

import (
	"context"
	"strings"
	"sync"
	"time"

	"slices"

	"manifold/internal/auth"
	"manifold/internal/playground/experiment"
)

// InMemoryRunStore provides an in-memory implementation of RunStore for tests and the mock service.
type InMemoryRunStore struct {
	mu          sync.RWMutex
	experiments map[string]experiment.ExperimentSpec
	runs        map[string]Run
	runResults  map[string][]RunResult
}

// NewInMemoryRunStore constructs an empty run store.
func NewInMemoryRunStore() *InMemoryRunStore {
	return &InMemoryRunStore{
		experiments: make(map[string]experiment.ExperimentSpec),
		runs:        make(map[string]Run),
		runResults:  make(map[string][]RunResult),
	}
}

// CreateExperiment stores the spec keyed by ID.
func (s *InMemoryRunStore) CreateExperiment(_ context.Context, spec experiment.ExperimentSpec) (experiment.ExperimentSpec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.experiments[spec.ID] = spec
	return spec, nil
}

// GetExperiment returns the spec if found.
func (s *InMemoryRunStore) GetExperiment(ctx context.Context, id string) (experiment.ExperimentSpec, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.experiments[id]
	if !ok {
		return experiment.ExperimentSpec{}, false, nil
	}
	if u, okU := auth.CurrentUser(ctx); okU && u != nil {
		if spec.OwnerID != u.ID {
			return experiment.ExperimentSpec{}, false, nil
		}
	}
	return spec, true, nil
}

// ListExperiments returns all experiments sorted by creation time descending.
func (s *InMemoryRunStore) ListExperiments(ctx context.Context) ([]experiment.ExperimentSpec, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]experiment.ExperimentSpec, 0, len(s.experiments))
	for _, spec := range s.experiments {
		if u, ok := auth.CurrentUser(ctx); ok && u != nil {
			if spec.OwnerID != u.ID {
				continue
			}
		}
		items = append(items, spec)
	}
	slices.SortFunc(items, func(a, b experiment.ExperimentSpec) int {
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return items, nil
}

// CreateRun persists the run metadata.
func (s *InMemoryRunStore) CreateRun(_ context.Context, run Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	return run, nil
}

// UpdateRunStatus updates the run status fields.
func (s *InMemoryRunStore) UpdateRunStatus(_ context.Context, id string, status RunStatus, endedAt time.Time, errMsg string, metrics map[string]float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return nil
	}
	run.Status = status
	run.EndedAt = endedAt
	run.Error = errMsg
	if metrics != nil {
		run.Metrics = cloneMetrics(metrics)
	}
	s.runs[id] = run
	return nil
}

// AppendResults records the run results.
func (s *InMemoryRunStore) AppendResults(_ context.Context, runID string, results []RunResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]RunResult, len(results))
	copy(copied, results)
	s.runResults[runID] = append(s.runResults[runID], copied...)
	return nil
}

// ListRuns returns runs for an experiment ordered by creation time desc.
func (s *InMemoryRunStore) ListRuns(ctx context.Context, experimentID string) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Run
	var uid int64
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		uid = u.ID
	}
	for _, run := range s.runs {
		if run.ExperimentID == experimentID {
			if uid != 0 && run.OwnerID != uid {
				continue
			}
			out = append(out, run)
		}
	}
	slices.SortFunc(out, func(a, b Run) int {
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return 1
		}
		return 0
	})
	return out, nil
}

// ListRunResults returns the stored results for a run.
func (s *InMemoryRunStore) ListRunResults(ctx context.Context, runID string) ([]RunResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Enforce ownership via the run
	if run, ok := s.runs[runID]; ok {
		if u, okU := auth.CurrentUser(ctx); okU && u != nil {
			if run.OwnerID != u.ID {
				return nil, nil
			}
		}
	}
	items := s.runResults[runID]
	if len(items) == 0 {
		return nil, nil
	}
	out := make([]RunResult, len(items))
	copy(out, items)
	slices.SortFunc(out, func(a, b RunResult) int {
		if a.RowID < b.RowID {
			return -1
		}
		if a.RowID > b.RowID {
			return 1
		}
		if a.VariantID < b.VariantID {
			return -1
		}
		if a.VariantID > b.VariantID {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

// DeleteExperiment removes an experiment spec and any runs/results for it.
func (s *InMemoryRunStore) DeleteExperiment(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.experiments, id)
	// delete runs and their results
	for runID, run := range s.runs {
		if run.ExperimentID == id {
			delete(s.runs, runID)
			delete(s.runResults, runID)
		}
	}
	return nil
}

func cloneMetrics(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
