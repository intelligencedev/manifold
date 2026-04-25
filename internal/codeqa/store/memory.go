package store

import (
	"context"
	"sort"
	"sync"

	"manifold/internal/codeqa"
)

type MemoryStore struct {
	mu     sync.RWMutex
	runs   map[int64]map[string]codeqa.RunResult
	events map[int64]map[string][]codeqa.RunEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		runs:   make(map[int64]map[string]codeqa.RunResult),
		events: make(map[int64]map[string][]codeqa.RunEvent),
	}
}

func (s *MemoryStore) Init(_ context.Context) error { return nil }

func (s *MemoryStore) Save(_ context.Context, userID int64, run codeqa.RunResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[userID]; !ok {
		s.runs[userID] = make(map[string]codeqa.RunResult)
	}
	s.runs[userID][run.RunID] = run
	return nil
}

func (s *MemoryStore) Get(_ context.Context, userID int64, runID string) (codeqa.RunResult, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[userID][runID]
	return run, ok, nil
}

func (s *MemoryStore) List(_ context.Context, userID int64, limit int) ([]codeqa.RunResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userRuns := s.runs[userID]
	out := make([]codeqa.RunResult, 0, len(userRuns))
	for _, run := range userRuns {
		out = append(out, run)
	}
	sort.Slice(out, func(i int, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) AppendEvent(_ context.Context, userID int64, event codeqa.RunEvent) (codeqa.RunEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[userID]; !ok {
		s.events[userID] = make(map[string][]codeqa.RunEvent)
	}
	event.Sequence = int64(len(s.events[userID][event.RunID]) + 1)
	s.events[userID][event.RunID] = append(s.events[userID][event.RunID], event)
	return event, nil
}

func (s *MemoryStore) ListEvents(_ context.Context, userID int64, runID string) ([]codeqa.RunEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.events[userID][runID]
	out := make([]codeqa.RunEvent, len(src))
	copy(out, src)
	return out, nil
}
