package flow

// Workflow is the runtime definition for Flow v2.
// It intentionally excludes canvas/layout metadata.
type Workflow struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Keywords    []string         `json:"keywords,omitempty"`
	ProjectID   string           `json:"project_id,omitempty"`
	Trigger     Trigger          `json:"trigger"`
	Nodes       []Node           `json:"nodes"`
	Edges       []Edge           `json:"edges,omitempty"`
	Settings    WorkflowSettings `json:"settings,omitempty"`
}

// Trigger defines how workflow execution starts.
type Trigger struct {
	Type     TriggerType      `json:"type"`
	Schedule *ScheduleTrigger `json:"schedule,omitempty"`
	Webhook  *WebhookTrigger  `json:"webhook,omitempty"`
	Event    *EventTrigger    `json:"event,omitempty"`
}

type TriggerType string

const (
	TriggerTypeManual   TriggerType = "manual"
	TriggerTypeSchedule TriggerType = "schedule"
	TriggerTypeWebhook  TriggerType = "webhook"
	TriggerTypeEvent    TriggerType = "event"
)

type ScheduleTrigger struct {
	// Cron uses standard 5-field cron syntax.
	Cron string `json:"cron"`
}

type WebhookTrigger struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type EventTrigger struct {
	Name string `json:"name"`
}

// Node is a unit of work in the DAG.
type Node struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Kind          NodeKind                `json:"kind"`
	Type          string                  `json:"type"`
	Guard         string                  `json:"guard,omitempty"`
	Tool          string                  `json:"tool,omitempty"`
	PublishResult bool                    `json:"publish_result,omitempty"`
	PublishMode   string                  `json:"publish_mode,omitempty"`
	Inputs        map[string]InputBinding `json:"inputs,omitempty"`
	Execution     NodeExecution           `json:"execution,omitempty"`
}

type NodeKind string

const (
	NodeKindAction NodeKind = "action"
	NodeKindLogic  NodeKind = "logic"
	NodeKindData   NodeKind = "data"
)

// InputBinding provides an explicit choice between literal and expression.
type InputBinding struct {
	Literal    any    `json:"literal,omitempty"`
	Expression string `json:"expression,omitempty"`
}

// NodeExecution configures runtime behavior for an individual node.
type NodeExecution struct {
	Timeout string        `json:"timeout,omitempty"`
	Retries RetryPolicy   `json:"retries,omitempty"`
	OnError ErrorStrategy `json:"on_error,omitempty"`
}

type RetryPolicy struct {
	Max     int             `json:"max,omitempty"`
	Backoff BackoffStrategy `json:"backoff,omitempty"`
}

type BackoffStrategy string

const (
	BackoffNone        BackoffStrategy = ""
	BackoffFixed       BackoffStrategy = "fixed"
	BackoffExponential BackoffStrategy = "exponential"
)

type ErrorStrategy string

const (
	ErrorStrategyFail     ErrorStrategy = "fail"
	ErrorStrategyContinue ErrorStrategy = "continue"
)

// Edge connects source output port to target input port.
// Optional field mappings support edge-level data mapping.
type Edge struct {
	ID      string         `json:"id,omitempty"`
	Source  PortRef        `json:"source"`
	Target  PortRef        `json:"target"`
	Mapping []FieldMapping `json:"mapping,omitempty"`
}

type PortRef struct {
	NodeID string `json:"node_id"`
	Port   string `json:"port"`
}

type FieldMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type WorkflowSettings struct {
	MaxConcurrency   int           `json:"max_concurrency,omitempty"`
	DefaultExecution NodeExecution `json:"default_execution,omitempty"`
}

// WorkflowCanvas stores editor-only metadata separate from runtime definition.
type WorkflowCanvas struct {
	Nodes     map[string]CanvasNode `json:"nodes,omitempty"`
	Parents   map[string]string     `json:"parents,omitempty"`
	Groups    []CanvasGroup         `json:"groups,omitempty"`
	Notes     []CanvasNote          `json:"notes,omitempty"`
	EdgeStyle string                `json:"edge_style,omitempty"`
}

type CanvasNode struct {
	X         float64  `json:"x"`
	Y         float64  `json:"y"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
	Collapsed *bool    `json:"collapsed,omitempty"`
	Label     string   `json:"label,omitempty"`
}

type CanvasGroup struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Color     string `json:"color,omitempty"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

type CanvasNote struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Note  string `json:"note,omitempty"`
	Color string `json:"color,omitempty"`
}
