package flow

import "time"

type WorkflowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ListWorkflowsResponse struct {
	Workflows []WorkflowSummary `json:"workflows"`
}

type GetWorkflowResponse struct {
	Workflow Workflow       `json:"workflow"`
	Canvas   WorkflowCanvas `json:"canvas,omitempty"`
}

type PutWorkflowRequest struct {
	Workflow Workflow       `json:"workflow"`
	Canvas   WorkflowCanvas `json:"canvas,omitempty"`
}

type ValidateRequest struct {
	Workflow Workflow `json:"workflow"`
}

type ValidateResponse struct {
	Valid       bool         `json:"valid"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Plan        *Plan        `json:"plan,omitempty"`
}

type RunRequest struct {
	WorkflowID string         `json:"workflow_id"`
	Input      map[string]any `json:"input,omitempty"`
	ProjectID  string         `json:"project_id,omitempty"`
}

type RunResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type RunEventType string

const (
	RunEventTypeRunStarted     RunEventType = "run_started"
	RunEventTypeRunCompleted   RunEventType = "run_completed"
	RunEventTypeRunFailed      RunEventType = "run_failed"
	RunEventTypeNodeStarted    RunEventType = "node_started"
	RunEventTypeNodeCompleted  RunEventType = "node_completed"
	RunEventTypeNodeFailed     RunEventType = "node_failed"
	RunEventTypeNodeSkipped    RunEventType = "node_skipped"
	RunEventTypeNodeRetrying   RunEventType = "node_retrying"
	RunEventTypeRunCancelled   RunEventType = "run_cancelled"
	RunEventTypeNodeOutputDiff RunEventType = "node_output_diff"
)

type RunEvent struct {
	RunID      string         `json:"run_id"`
	Sequence   int64          `json:"sequence"`
	Type       RunEventType   `json:"type"`
	NodeID     string         `json:"node_id,omitempty"`
	Status     string         `json:"status,omitempty"`
	Message    string         `json:"message,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	OccurredAt time.Time      `json:"occurred_at"`
}
