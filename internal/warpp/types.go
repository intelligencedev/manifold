package warpp

import "encoding/json"

// Workflow is a typed representation of a natural-language workflow with
// optional guards and tool references per step.
type Workflow struct {
	Intent      string      `json:"intent"`
	Description string      `json:"description"`
	Keywords    []string    `json:"keywords"`
	Steps       []Step      `json:"steps"`
	UI          *WorkflowUI `json:"ui,omitempty"`
	// Optional execution hints
	MaxConcurrency int  `json:"max_concurrency,omitempty"`
	FailFast       bool `json:"fail_fast,omitempty"`
}

// WorkflowUI holds optional editor metadata (for example, node layout).
type WorkflowUI struct {
	Layout  map[string]NodeLayout `json:"layout,omitempty"`
	Parents map[string]string     `json:"parents,omitempty"`
	Groups  []GroupUIEntry        `json:"groups,omitempty"`
}

// GroupUIEntry represents a group container node in the UI.
type GroupUIEntry struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

// NodeLayout captures the 2D position and optional size of a node on the editor canvas.
type NodeLayout struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
}

type Step struct {
	ID    string   `json:"id"`
	Text  string   `json:"text"`
	Guard string   `json:"guard,omitempty"`
	Tool  *ToolRef `json:"tool,omitempty"`
	// PublishResult controls whether the result payload of this step should
	// be published to Kafka (or any configured publisher) as it completes.
	PublishResult bool `json:"publish_result,omitempty"`
	// DAG: optional list of step IDs this step depends on
	DependsOn []string `json:"depends_on,omitempty"`
	// Execution hints
	ContinueOnError bool   `json:"continue_on_error,omitempty"`
	PublishMode     string `json:"publish_mode,omitempty"` // "immediate" (default) | "topo"
	Timeout         string `json:"timeout,omitempty"`      // e.g., "30s"
	Retries         int    `json:"retries,omitempty"`
}

type ToolRef struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// StepTrace captures the rendered arguments and outputs for a step during
// execution. It is used by the editor to surface runtime attribute values.
type StepTrace struct {
	StepID       string          `json:"step_id"`
	Text         string          `json:"text,omitempty"`
	RenderedArgs map[string]any  `json:"rendered_args,omitempty"`
	Delta        Attrs           `json:"delta,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Status       string          `json:"status,omitempty"`
	Error        string          `json:"error,omitempty"`
}

// Attrs are user attributes discovered during personalization.
type Attrs map[string]any
