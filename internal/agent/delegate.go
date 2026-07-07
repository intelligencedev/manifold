package agent

import (
	"context"

	"manifold/internal/llm"
)

// AgentTrace captures lifecycle events for delegated agent runs (specialists
// or other orchestrators). UIs can use these events to render nested agent
// interactions.
type AgentTrace struct {
	Type           string
	Agent          string
	Team           string
	Model          string
	CallID         string
	ParentCallID   string
	Depth          int
	Role           string
	Content        string
	Title          string
	ToolName       string
	ToolTitle      string
	Args           string
	Data           string
	ToolID         string
	Error          string
	ThoughtSummary string
}

// AgentTracer receives trace events emitted during delegated agent execution.
type AgentTracer interface {
	Trace(AgentTrace)
}

// DelegateRequest describes a delegated agent invocation.
type DelegateRequest struct {
	AgentName             string
	Prompt                string
	History               []llm.Message
	EnableTools           *bool
	MaxSteps              int
	TimeoutSeconds        int
	ProjectID             string
	ObjectiveID           string
	SessionID             string
	UserID                int64
	CallID                string
	ParentCallID          string
	Depth                 int
	DisableEvolvingMemory bool
	DisableBeliefMemory   bool
}

// Delegator executes delegated agent runs, optionally streaming trace events
// via AgentTracer. The return value is the final assistant text that should be
// appended to the parent agent loop as the tool result.
type Delegator interface {
	Run(ctx context.Context, req DelegateRequest, tracer AgentTracer) (string, error)
}

// TeamDelegateRequest describes a delegated team invocation.
type TeamDelegateRequest struct {
	TeamName              string
	Prompt                string
	History               []llm.Message
	TimeoutSeconds        int
	TimeoutMS             int
	ProjectID             string
	ObjectiveID           string
	SessionID             string
	UserID                int64
	CallID                string
	ParentCallID          string
	Depth                 int
	DisableEvolvingMemory bool
	DisableBeliefMemory   bool
}

// TeamDelegator executes delegated team runs, optionally streaming trace events
// via AgentTracer. The return value is the final team orchestrator output.
type TeamDelegator interface {
	RunTeam(ctx context.Context, req TeamDelegateRequest, tracer AgentTracer) (string, error)
}
