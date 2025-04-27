package agent

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	openai "github.com/sashabaranov/go-openai"
)

// InternalAgentHandler handles HTTP requests for agent operations
type InternalAgentHandler struct {
	Engine *Engine
}

// NewInternalAgentHandler creates a new InternalAgentHandler with the given config
func NewInternalAgentHandler(config interface{}) (*InternalAgentHandler, error) {
	// Type assert config to get the appropriate configuration fields
	mainConfig, ok := config.(*Config)
	if !ok {
		return nil, echo.NewHTTPError(500, "invalid config type")
	}

	// Create tool registry
	registry := NewRegistry()

	// Register built-in tools
	RegisterBuiltinTools(registry)

	// Create OpenAI client for the planner
	client := openai.NewClient(mainConfig.Completions.APIKey)

	// Create planner with system template
	planner := &LLMPlanner{
		Client:    client,
		ToolSpecs: registry.Spec(),
		SystemTpl: `You are the Planner. Return ONLY a JSON array.
Each element must have: description, tool (string|null), args (object).
Available tools: %s`,
	}

	// Create an LLM-based critic
	critic := NewLLMCritic(client)

	// Create the engine
	engine := &Engine{
		Planner:       planner,
		Executor:      &ConcurrentExecutor{Registry: registry},
		Memory:        NewRingMemory(64),
		Critic:        critic,
		Tracer:        NewOTELTracer(),
		Success:       NoErrorSuccess{Window: 1},
		PlanTimeout:   15 * time.Second,
		ExecTimeout:   30 * time.Second,
		CriticTimeout: 10 * time.Second,
		MaxIters:      5,
	}

	return &InternalAgentHandler{Engine: engine}, nil
}

func (h *InternalAgentHandler) PlanHandler(c echo.Context) error {
	// For GET requests, extract goal from query parameters
	goal := c.QueryParam("goal")
	if goal == "" {
		// If goal is not in query params, try to parse from JSON body
		var req struct {
			Goal string `json:"goal"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(400, map[string]string{"error": "invalid request"})
		}
		goal = req.Goal
	}

	if goal == "" {
		return c.JSON(400, map[string]string{"error": "goal is required"})
	}

	steps, err := h.Engine.Planner.Plan(c.Request().Context(), goal, nil)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, steps)
}

func (h *InternalAgentHandler) RunHandler(c echo.Context) error {
	var req struct {
		Goal string `json:"goal"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request"})
	}
	answer, err := h.Engine.Run(c.Request().Context(), req.Goal)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, map[string]string{"answer": answer})
}

func (h *InternalAgentHandler) ExecuteHandler(c echo.Context) error {
	var step Step
	if err := c.Bind(&step); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid step payload"})
	}
	out, err := h.Engine.Executor.Execute(c.Request().Context(), step)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, map[string]interface{}{"result": out})
}

func (h *InternalAgentHandler) CritiqueHandler(c echo.Context) error {
	var req struct {
		History []Interaction `json:"history"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid request"})
	}
	critique, err := h.Engine.Critic.Critique(c.Request().Context(), req.History)
	if err != nil {
		return c.JSON(500, map[string]string{"error": err.Error()})
	}
	return c.JSON(200, critique)
}

// NoopCritic is a simple critic implementation that always approves
type NoopCritic struct{}

func (NoopCritic) Critique(_ context.Context, _ []Interaction) (Critique, error) {
	return Critique{Action: "approve"}, nil
}
