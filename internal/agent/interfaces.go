package agent

import "context"

// Planner turns a user goal into an ordered work-queue of Steps.
type Planner interface {
	Plan(ctx context.Context, goal string, relMem []MemoryItem) ([]Step, error)
}

// Executor performs a single Step (LLM call or Tool invocation).
type Executor interface {
	Execute(ctx context.Context, step Step) (any, error)
}

// Tool is an external capability.
type Tool interface {
	Describe() ToolSpec // For schema exposure / planner prompt
	Execute(ctx context.Context, args map[string]any) (any, error)
}

// Memory provides short- and long-term persistence.
type Memory interface {
	Recall(ctx context.Context, query string, k int) ([]MemoryItem, error)
	Store(ctx context.Context, item MemoryItem) error
}

// Critic inspects recent history and may suggest a patch.
type Critic interface {
	Critique(ctx context.Context, trace []Interaction) (Critique, error)
}

// Tracer emits structured traces/spans.
type Tracer interface {
	Start(ctx context.Context, name string, attrs map[string]any) (context.Context, func(err error))
}
