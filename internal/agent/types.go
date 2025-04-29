package agent

// Step is an atomic planned action.
type Step struct {
	ID          string // deterministic UUID for traceability
	Description string
	Tool        string         // "" → pure-LLM step
	Args        map[string]any // JSON-like
}

// Observation is the result of a Step.
type Observation struct {
	Step   Step
	Output any
	Err    error
}

// MemoryItem couples Step and Observation.
type MemoryItem struct {
	Step        Step
	Observation Observation
}

// Interaction is flattened for Critic input.
type Interaction struct {
	Step        Step
	Observation Observation
}

// ToolSpec fed into the planner prompt.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Critique returned by Critic.
type Critique struct {
	Action string // "approve" | "revise"
	Fix    *Step  // non-nil if Action == "revise"
	Reason string
}
