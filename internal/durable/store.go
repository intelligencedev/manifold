package durable

import (
	"context"
	"encoding/json"
	"time"
)

type Store interface {
	Init(ctx context.Context) error
	SpawnTask(ctx context.Context, req SpawnRequest) (Task, bool, error)
	GetTask(ctx context.Context, userID int64, taskID string) (Task, bool, error)
	ListTasks(ctx context.Context, userID int64, filter TaskListFilter) ([]Task, error)
	ListTaskEvents(ctx context.Context, userID int64, taskID string, afterSequence int64) ([]Event, TaskStatus, bool, error)
	AppendTaskEvent(ctx context.Context, taskID string, name string, payload map[string]any) (Event, error)
	AppendTaskEventOnce(ctx context.Context, taskID string, eventKey string, name string, payload map[string]any) (Event, error)
	EmitEvent(ctx context.Context, userID int64, queue string, name string, payload map[string]any) (Event, error)
	ClaimNext(ctx context.Context, queues []string, workerID string, lease time.Duration) (Task, Run, bool, error)
	Heartbeat(ctx context.Context, taskID, runID, workerID string, leaseUntil time.Time) error
	CompleteTask(ctx context.Context, taskID, runID string, result json.RawMessage) error
	FailTask(ctx context.Context, taskID, runID string, failure json.RawMessage, errText string, nextAttemptAt time.Time) error
	MarkTaskWaiting(ctx context.Context, taskID, runID string) error
	CancelTask(ctx context.Context, userID int64, taskID string) error
	CancelTaskTree(ctx context.Context, userID int64, taskID string) ([]string, error)
	RetryTask(ctx context.Context, userID int64, taskID string, resetCheckpoints bool) (Task, error)
	GetCheckpoint(ctx context.Context, taskID, stepKey string) (json.RawMessage, bool, error)
	SaveCheckpoint(ctx context.Context, taskID, stepKey string, value json.RawMessage) (json.RawMessage, error)
	CreateWait(ctx context.Context, wait Wait) (Wait, error)
	GetWait(ctx context.Context, waitID string) (Wait, bool, error)
	FireDueTimers(ctx context.Context, now time.Time, limit int) (int, error)
	WakeChildWaits(ctx context.Context, childTaskID string) error
	QueueStats(ctx context.Context) ([]QueueStats, error)
	Close()
}
