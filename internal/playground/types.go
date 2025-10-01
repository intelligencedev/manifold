package playground

import (
	"time"

	"manifold/internal/playground/experiment"
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
	ID           string             `json:"id"`
	ExperimentID string             `json:"experimentId"`
	Plan         experiment.RunPlan `json:"plan"`
	Status       RunStatus          `json:"status"`
	CreatedAt    time.Time          `json:"createdAt"`
	StartedAt    time.Time          `json:"startedAt,omitempty"`
	EndedAt      time.Time          `json:"endedAt,omitempty"`
	Error        string             `json:"error,omitempty"`
	Metrics      map[string]float64 `json:"metrics,omitempty"`
}

// RunResult stores the per-row evaluation outcome of a run.
type RunResult struct {
	ID              string             `json:"id"`
	RunID           string             `json:"runId"`
	RowID           string             `json:"rowId"`
	VariantID       string             `json:"variantId"`
	PromptVersionID string             `json:"promptVersionId,omitempty"`
	Model           string             `json:"model,omitempty"`
	Rendered        string             `json:"rendered,omitempty"`
	Output          string             `json:"output,omitempty"`
	ProviderName    string             `json:"providerName,omitempty"`
	Tokens          int                `json:"tokens,omitempty"`
	Latency         time.Duration      `json:"latency,omitempty"`
	Artifacts       map[string]string  `json:"artifacts,omitempty"`
	Scores          map[string]float64 `json:"scores,omitempty"`
	Expected        any                `json:"expected,omitempty"`
}
