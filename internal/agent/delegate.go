package agent

import (
	"context"

	"manifold/internal/llm"
)

// AgentTrace captures lifecycle events for delegated agent runs (specialists
// or other orchestrators). UIs can use these events to render nested agent
// interactions.
type AgentTrace struct {
	Type         string
	Agent        string
	Model        string
	CallID       string
	ParentCallID string
	Depth        int
	Role         string
	Content      string
	Title        string
	Args         string
	Data         string
	ToolID       string
	Error        string
}

// AgentTracer receives trace events emitted during delegated agent execution.
type AgentTracer interface {
	Trace(AgentTrace)
}

// DelegateRequest describes a delegated agent invocation.
type DelegateRequest struct {
	AgentName      string
	Prompt         string
	History        []llm.Message
	EnableTools    *bool
	MaxSteps       int
	TimeoutSeconds int
	ProjectID      string
	UserID         int64
	CallID         string
	ParentCallID   string
	Depth          int
}

// Delegator executes delegated agent runs, optionally streaming trace events
// via AgentTracer. The return value is the final assistant text that should be
// appended to the parent agent loop as the tool result.
type Delegator interface {
	Run(ctx context.Context, req DelegateRequest, tracer AgentTracer) (string, error)
}
