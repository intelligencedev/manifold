package agent

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// MockPlanner implements the Planner interface for testing
type MockPlanner struct{}

func (MockPlanner) Plan(_ context.Context, goal string, _ []MemoryItem) ([]Step, error) {
	return []Step{
		{
			ID:          uuid.NewString(),
			Description: "Convert the text to uppercase",
			Tool:        "upper",
			Args:        map[string]any{"text": goal},
		},
	}, nil
}

// MockExecutor implements the Executor interface for testing
type MockExecutor struct{}

func (MockExecutor) Execute(_ context.Context, step Step) (any, error) {
	if step.Tool == "upper" {
		text, ok := step.Args["text"].(string)
		if !ok {
			return nil, ErrMissingParameter("text parameter must be a string")
		}
		return strings.ToUpper(text), nil
	}
	return "mock execution result", nil
}

// MockTool implements the Tool interface for testing
type MockTool struct {
	DescribeFunc func() ToolSpec
	ExecuteFunc  func(ctx context.Context, args map[string]any) (any, error)
}

func (t MockTool) Describe() ToolSpec {
	if t.DescribeFunc != nil {
		return t.DescribeFunc()
	}
	return ToolSpec{
		Name:        "mockTool",
		Description: "A mock tool for testing",
		Parameters: map[string]any{
			"param": map[string]string{
				"type":        "string",
				"description": "A parameter",
			},
		},
	}
}

func (t MockTool) Execute(ctx context.Context, args map[string]any) (any, error) {
	if t.ExecuteFunc != nil {
		return t.ExecuteFunc(ctx, args)
	}
	return "mock result", nil
}

// MockMemory implements the Memory interface for testing
type MockMemory struct {
	Items []MemoryItem
}

func (m *MockMemory) Recall(_ context.Context, _ string, k int) ([]MemoryItem, error) {
	if len(m.Items) == 0 {
		return nil, nil
	}

	if k >= len(m.Items) {
		return m.Items, nil
	}

	return m.Items[len(m.Items)-k:], nil
}

func (m *MockMemory) Store(_ context.Context, item MemoryItem) error {
	m.Items = append(m.Items, item)
	return nil
}

// MockCritic implements the Critic interface for testing
type MockCritic struct {
	CritiqueFunc func(ctx context.Context, trace []Interaction) (Critique, error)
}

func (c MockCritic) Critique(ctx context.Context, trace []Interaction) (Critique, error) {
	if c.CritiqueFunc != nil {
		return c.CritiqueFunc(ctx, trace)
	}
	return Critique{Action: "approve"}, nil
}
