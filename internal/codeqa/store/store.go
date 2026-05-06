package store

import (
	"context"

	"manifold/internal/codeqa"
)

type CodeQAStore interface {
	Init(ctx context.Context) error
	Save(ctx context.Context, userID int64, run codeqa.RunResult) error
	Get(ctx context.Context, userID int64, runID string) (codeqa.RunResult, bool, error)
	List(ctx context.Context, userID int64, limit int) ([]codeqa.RunResult, error)
	AppendEvent(ctx context.Context, userID int64, event codeqa.RunEvent) (codeqa.RunEvent, error)
	ListEvents(ctx context.Context, userID int64, runID string) ([]codeqa.RunEvent, error)
}
