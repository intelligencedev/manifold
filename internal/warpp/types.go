package warpp

// Workflow is a typed representation of a natural-language workflow with
// optional guards and tool references per step.
type Workflow struct {
	Intent      string   `json:"intent"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Steps       []Step   `json:"steps"`
}

type Step struct {
	ID    string   `json:"id"`
	Text  string   `json:"text"`
	Guard string   `json:"guard,omitempty"`
	Tool  *ToolRef `json:"tool,omitempty"`
	// PublishResult controls whether the result payload of this step should
	// be published to Kafka (or any configured publisher) as it completes.
	PublishResult bool `json:"publish_result,omitempty"`
}

type ToolRef struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

// Attrs are user attributes discovered during personalization.
type Attrs map[string]any
