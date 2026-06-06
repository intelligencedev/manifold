package store

import (
	"context"
	"maps"
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
	s.runs[userID][run.RunID] = cloneRunResult(run)
	return nil
}

func (s *MemoryStore) Get(_ context.Context, userID int64, runID string) (codeqa.RunResult, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[userID][runID]
	if !ok {
		return codeqa.RunResult{}, false, nil
	}
	return cloneRunResult(run), true, nil
}

func (s *MemoryStore) List(_ context.Context, userID int64, limit int) ([]codeqa.RunResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userRuns := s.runs[userID]
	out := make([]codeqa.RunResult, 0, len(userRuns))
	for _, run := range userRuns {
		out = append(out, cloneRunResult(run))
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
	cloned := cloneRunEvent(event)
	s.events[userID][event.RunID] = append(s.events[userID][event.RunID], cloned)
	return cloned, nil
}

func (s *MemoryStore) ListEvents(_ context.Context, userID int64, runID string) ([]codeqa.RunEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.events[userID][runID]
	out := make([]codeqa.RunEvent, len(src))
	for i, event := range src {
		out[i] = cloneRunEvent(event)
	}
	return out, nil
}

func cloneRunResult(in codeqa.RunResult) codeqa.RunResult {
	out := in
	out.Diff.Files = cloneChangedFiles(in.Diff.Files)
	out.Diff.SourceTrees = maps.Clone(in.Diff.SourceTrees)
	out.Gates = cloneGateResults(in.Gates)
	out.Judges = cloneJudgeVerdicts(in.Judges)
	out.Aggregate.HardFailures = append([]string(nil), in.Aggregate.HardFailures...)
	out.Artifacts = maps.Clone(in.Artifacts)
	return out
}

func cloneChangedFiles(in []codeqa.ChangedFile) []codeqa.ChangedFile {
	if len(in) == 0 {
		return nil
	}
	out := make([]codeqa.ChangedFile, len(in))
	for i, file := range in {
		out[i] = file
		out[i].RelatedTests = append([]string(nil), file.RelatedTests...)
	}
	return out
}

func cloneGateResults(in []codeqa.GateResult) []codeqa.GateResult {
	if len(in) == 0 {
		return nil
	}
	out := make([]codeqa.GateResult, len(in))
	for i, gate := range in {
		out[i] = gate
		out[i].Metrics = maps.Clone(gate.Metrics)
	}
	return out
}

func cloneJudgeVerdicts(in []codeqa.JudgeVerdict) []codeqa.JudgeVerdict {
	if len(in) == 0 {
		return nil
	}
	out := make([]codeqa.JudgeVerdict, len(in))
	for i, verdict := range in {
		out[i] = verdict
		out[i].Scores = maps.Clone(verdict.Scores)
		out[i].BlockingConcerns = append([]string(nil), verdict.BlockingConcerns...)
		out[i].Evidence = append([]string(nil), verdict.Evidence...)
	}
	return out
}

func cloneRunEvent(in codeqa.RunEvent) codeqa.RunEvent {
	out := in
	out.Payload = maps.Clone(in.Payload)
	return out
}
