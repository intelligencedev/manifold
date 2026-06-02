package durable

import (
	"encoding/json"
	"time"
)

const (
	DefaultQueue = "default"

	DefaultEventListLimit = 200
	MaxEventListLimit     = 1000
)

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusWaiting   TaskStatus = "waiting"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusWaiting   RunStatus = "waiting"
	RunStatusCancelled RunStatus = "cancelled"
)

type WaitKind string

const (
	WaitKindTimer WaitKind = "timer"
	WaitKindEvent WaitKind = "event"
	WaitKindChild WaitKind = "child"
)

type RetryPolicy struct {
	MaxAttempts      int    `json:"max_attempts,omitempty"`
	Backoff          string `json:"backoff,omitempty"`
	BaseDelaySeconds int    `json:"base_delay_seconds,omitempty"`
	MaxDelaySeconds  int    `json:"max_delay_seconds,omitempty"`
}

type SpawnRequest struct {
	Queue          string         `json:"queue,omitempty"`
	Name           string         `json:"name"`
	UserID         int64          `json:"user_id,omitempty"`
	Params         map[string]any `json:"params,omitempty"`
	Headers        map[string]any `json:"headers,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	ParentTaskID   string         `json:"parent_task_id,omitempty"`
	ParentRunID    string         `json:"parent_run_id,omitempty"`
	RetryPolicy    RetryPolicy    `json:"retry_policy"`
	AvailableAt    time.Time      `json:"available_at"`
}

type SpawnResult struct {
	TaskID  string `json:"task_id"`
	RunID   string `json:"run_id,omitempty"`
	Created bool   `json:"created"`
}

type TaskListFilter struct {
	Queue  string
	Status TaskStatus
	Name   string
	Limit  int
}

type EventListFilter struct {
	AfterSequence  int64
	BeforeSequence int64
	Limit          int
}

type EventPage struct {
	Events        []Event
	Status        TaskStatus
	Found         bool
	FirstSequence int64
	LastSequence  int64
	HasMoreBefore bool
	HasMoreAfter  bool
	Limit         int
}

type Task struct {
	ID             string          `json:"id"`
	Queue          string          `json:"queue"`
	Name           string          `json:"name"`
	UserID         int64           `json:"user_id"`
	Status         TaskStatus      `json:"status"`
	Params         map[string]any  `json:"params,omitempty"`
	Headers        map[string]any  `json:"headers,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	ParentTaskID   string          `json:"parent_task_id,omitempty"`
	ParentRunID    string          `json:"parent_run_id,omitempty"`
	RetryPolicy    RetryPolicy     `json:"retry_policy"`
	Attempt        int             `json:"attempt"`
	AvailableAt    time.Time       `json:"available_at"`
	Result         json.RawMessage `json:"result,omitempty"`
	Failure        json.RawMessage `json:"failure,omitempty"`
	Error          string          `json:"error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

type Run struct {
	ID          string     `json:"id"`
	TaskID      string     `json:"task_id"`
	Attempt     int        `json:"attempt"`
	Status      RunStatus  `json:"status"`
	WorkerID    string     `json:"worker_id,omitempty"`
	LeaseUntil  time.Time  `json:"lease_until"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type Event struct {
	ID         int64          `json:"id"`
	TaskID     string         `json:"task_id,omitempty"`
	Queue      string         `json:"queue,omitempty"`
	Name       string         `json:"name"`
	Sequence   int64          `json:"sequence"`
	EventKey   string         `json:"event_key,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
}

type Wait struct {
	ID          string     `json:"id"`
	TaskID      string     `json:"task_id"`
	RunID       string     `json:"run_id,omitempty"`
	Kind        WaitKind   `json:"kind"`
	EventName   string     `json:"event_name,omitempty"`
	ChildTaskID string     `json:"child_task_id,omitempty"`
	WakeAt      *time.Time `json:"wake_at,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	FiredAt     *time.Time `json:"fired_at,omitempty"`
}

type QueueStats struct {
	Queue     string `json:"queue"`
	Queued    int64  `json:"queued"`
	Running   int64  `json:"running"`
	Waiting   int64  `json:"waiting"`
	Completed int64  `json:"completed"`
	Failed    int64  `json:"failed"`
	Cancelled int64  `json:"cancelled"`
}

type ResultSnapshot struct {
	TaskID  string          `json:"task_id"`
	State   TaskStatus      `json:"state"`
	Result  json.RawMessage `json:"result,omitempty"`
	Failure json.RawMessage `json:"failure,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func (s ResultSnapshot) IsTerminal() bool {
	return s.State == TaskStatusCompleted || s.State == TaskStatusFailed || s.State == TaskStatusCancelled
}

func (s ResultSnapshot) DecodeResult(dst any) error {
	if len(s.Result) == 0 {
		return nil
	}
	return json.Unmarshal(s.Result, dst)
}

func (s ResultSnapshot) DecodeFailure(dst any) error {
	if len(s.Failure) == 0 {
		return nil
	}
	return json.Unmarshal(s.Failure, dst)
}
