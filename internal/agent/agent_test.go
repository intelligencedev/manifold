package agent

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register("upper", UpperTool{})

	specs := r.Spec()
	if len(specs) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(specs))
	}

	if specs[0].Name != "upper" {
		t.Errorf("expected tool name 'upper', got '%s'", specs[0].Name)
	}

	// Test tool execution
	result, err := r.Execute(context.Background(), "upper", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	if result != "HELLO" {
		t.Errorf("expected 'HELLO', got '%v'", result)
	}

	// Test executing unknown tool
	_, err = r.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Errorf("expected error for unknown tool, got nil")
	}
}

func TestMemory(t *testing.T) {
	m := NewRingMemory(3)

	// Store 4 items, should only keep the last 3
	for i := 1; i <= 4; i++ {
		step := Step{
			ID:          uuid.NewString(),
			Description: "test step",
			Tool:        "test",
			Args:        map[string]any{"value": i},
		}
		obs := Observation{
			Step:   step,
			Output: i,
		}
		err := m.Store(context.Background(), MemoryItem{Step: step, Observation: obs})
		if err != nil {
			t.Fatalf("store failed: %v", err)
		}
	}

	// Recall 2 items
	items, err := m.Recall(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// The items should be the last 2 stored (items 3 and 4)
	if items[0].Observation.Output.(int) != 3 || items[1].Observation.Output.(int) != 4 {
		t.Errorf("incorrect items recalled: %v", items)
	}
}

func TestCritic(t *testing.T) {
	critic := &LLMCritic{}

	// Create a trace with an error in the last step
	errorStep := Step{
		ID:          uuid.NewString(),
		Description: "Upper case text",
		Tool:        "upper",
		Args:        map[string]any{}, // Missing the "text" parameter
	}

	errorTrace := []Interaction{
		{
			Step: errorStep,
			Observation: Observation{
				Step: errorStep,
				Err:  ErrMissingParameter("text parameter must be a string"),
			},
		},
	}

	critique, err := critic.Critique(context.Background(), errorTrace)
	if err != nil {
		t.Fatalf("critique failed: %v", err)
	}

	if critique.Action != "revise" {
		t.Errorf("expected action 'revise', got '%s'", critique.Action)
	}

	if critique.Fix == nil {
		t.Fatalf("expected non-nil fix")
	}

	if critique.Fix.Tool != "upper" {
		t.Errorf("expected fixed tool 'upper', got '%s'", critique.Fix.Tool)
	}

	// Test successful trace
	successStep := Step{
		ID:          uuid.NewString(),
		Description: "Upper case text",
		Tool:        "upper",
		Args:        map[string]any{"text": "hello"},
	}

	successTrace := []Interaction{
		{
			Step: successStep,
			Observation: Observation{
				Step:   successStep,
				Output: "HELLO",
			},
		},
	}

	critique, err = critic.Critique(context.Background(), successTrace)
	if err != nil {
		t.Fatalf("critique failed: %v", err)
	}

	if critique.Action != "approve" {
		t.Errorf("expected action 'approve', got '%s'", critique.Action)
	}
}

// Helper error creator
func ErrMissingParameter(msg string) error {
	return &missingParamError{msg}
}

type missingParamError struct {
	msg string
}

func (e *missingParamError) Error() string {
	return e.msg
}
