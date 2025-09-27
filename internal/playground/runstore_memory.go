package playground

import (
	"context"
	"sync"
	"time"

	"slices"

	"intelligence.dev/internal/playground/experiment"
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
func (s *InMemoryRunStore) GetExperiment(_ context.Context, id string) (experiment.ExperimentSpec, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.experiments[id]
	return spec, ok, nil
}

// CreateRun persists the run metadata.
func (s *InMemoryRunStore) CreateRun(_ context.Context, run Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	return run, nil
}

// UpdateRunStatus updates the run status fields.
func (s *InMemoryRunStore) UpdateRunStatus(_ context.Context, id string, status RunStatus, endedAt time.Time, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return nil
	}
	run.Status = status
	run.EndedAt = endedAt
	run.Error = errMsg
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
func (s *InMemoryRunStore) ListRuns(_ context.Context, experimentID string) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Run
	for _, run := range s.runs {
		if run.ExperimentID == experimentID {
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
