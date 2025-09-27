package playground

import (
	"time"

	"intelligence.dev/internal/playground/experiment"
)

// RunStatus models the lifecycle of an experiment run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCompleted RunStatus = "completed"
)

// Run captures a single execution of an ExperimentSpec.
type Run struct {
	ID           string
	ExperimentID string
	Plan         experiment.RunPlan
	Status       RunStatus
	CreatedAt    time.Time
	StartedAt    time.Time
	EndedAt      time.Time
	Error        string
	Metrics      map[string]float64
}

// RunResult stores the per-row evaluation outcome of a run.
type RunResult struct {
	ID              string
	RunID           string
	RowID           string
	VariantID       string
	PromptVersionID string
	Model           string
	Rendered        string
	Output          string
	ProviderName    string
	Tokens          int
	Latency         time.Duration
	Artifacts       map[string]string
	Scores          map[string]float64
	Expected        any
}
