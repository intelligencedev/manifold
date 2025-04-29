package main

import (
	"context"
	"fmt"
	"time"

	"manifold/internal/agent"

	"github.com/google/uuid"
)

func main() {
	// Create a registry and register the uppercase tool
	registry := agent.NewRegistry()
	registry.Register("upper", agent.UpperTool{})
	registry.Register("lower", agent.LowerTool{})
	registry.Register("countWords", agent.CountWordsTool{})

	// Create a simple mock planner for demonstration
	planner := &MockPlanner{}

	// Create a critic (we'll use the NoopCritic that always approves)
	critic := &agent.NoopCritic{}

	// Create the executor
	executor := &agent.ConcurrentExecutor{Registry: registry}

	// Create the engine
	engine := &agent.Engine{
		Planner:       planner,
		Executor:      executor,
		Memory:        agent.NewRingMemory(64),
		Critic:        critic,
		Tracer:        &agent.NullTracer{},
		Success:       agent.NoErrorSuccess{Window: 1},
		PlanTimeout:   5 * time.Second,
		ExecTimeout:   10 * time.Second,
		CriticTimeout: 5 * time.Second,
		MaxIters:      3,
	}

	// Run the agent with a goal
	goal := "Make 'hello world' uppercase"
	ctx := context.Background()
	result, err := engine.Run(ctx, goal)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Agent result:")
	fmt.Println(result)
}

// MockPlanner implements the Planner interface for demo purposes
type MockPlanner struct{}

func (MockPlanner) Plan(_ context.Context, goal string, _ []agent.MemoryItem) ([]agent.Step, error) {
	return []agent.Step{
		{
			ID:          uuid.NewString(),
			Description: "Convert the text to uppercase",
			Tool:        "upper",
			Args:        map[string]any{"text": goal},
		},
	}, nil
}
