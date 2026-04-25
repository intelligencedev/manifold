package service

import (
	"context"

	"manifold/internal/codeqa"
)

func (s *Service) GetRun(ctx context.Context, userID int64, runID string) (codeqa.RunResult, bool, error) {
	return s.store.Get(ctx, userID, runID)
}

func (s *Service) ListRuns(ctx context.Context, userID int64, limit int) ([]codeqa.RunResult, error) {
	return s.store.List(ctx, userID, limit)
}

func (s *Service) ListEvents(ctx context.Context, userID int64, runID string) ([]codeqa.RunEvent, error) {
	return s.store.ListEvents(ctx, userID, runID)
}
