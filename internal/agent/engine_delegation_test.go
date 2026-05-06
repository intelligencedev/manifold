package agent

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/llm"
	"manifold/internal/tools"
	"manifold/internal/tools/multitool"
)

type captureDelegator struct {
	req DelegateRequest
	ctx context.Context

	err error
	out string
}

func (d *captureDelegator) Run(ctx context.Context, req DelegateRequest, _ AgentTracer) (string, error) {
	d.ctx = ctx
	d.req = req
	return d.out, d.err
}

func TestRunDelegatedAgentCarriesSessionID(t *testing.T) {
	t.Parallel()

	spy := &captureDelegator{out: "delegated"}
	eng := &Engine{Delegator: spy, SessionID: "sess-delegate", ProjectID: "project-1", ObjectiveID: "objective-1", UserID: 7}
	args, err := json.Marshal(map[string]any{
		"agent_name": "writer",
		"prompt":     "draft this",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	payload := eng.runDelegatedAgent(context.Background(), llm.ToolCall{
		ID:   "tool-1",
		Name: "agent_call",
		Args: args,
	})

	if spy.req.SessionID != "sess-delegate" {
		t.Fatalf("expected session id sess-delegate, got %q", spy.req.SessionID)
	}
	if spy.req.ProjectID != "project-1" {
		t.Fatalf("expected project id project-1, got %q", spy.req.ProjectID)
	}
	if spy.req.ObjectiveID != "objective-1" {
		t.Fatalf("expected objective id objective-1, got %q", spy.req.ObjectiveID)
	}
	if spy.req.UserID != 7 {
		t.Fatalf("expected user id 7, got %d", spy.req.UserID)
	}
	if spy.req.AgentName != "writer" {
		t.Fatalf("expected agent writer, got %q", spy.req.AgentName)
	}
	if string(payload) == "" {
		t.Fatal("expected payload from delegated run")
	}
}

type tracingDelegator struct {
	req DelegateRequest
}

func (d *tracingDelegator) Run(ctx context.Context, req DelegateRequest, tracer AgentTracer) (string, error) {
	d.req = req
	if tracer != nil {
		tracer.Trace(AgentTrace{Type: "agent_start", Agent: req.AgentName, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth})
	}
	return "delegated", nil
}

type captureTracer struct {
	events []AgentTrace
}

func (t *captureTracer) Trace(ev AgentTrace) {
	t.events = append(t.events, ev)
}

func TestParallelToolAgentCallUsesDelegationTracer(t *testing.T) {
	t.Parallel()

	reg := tools.NewRegistry()
	reg.Register(multitool.NewParallel(reg))
	delegator := &tracingDelegator{}
	tracer := &captureTracer{}
	eng := &Engine{Tools: reg, Delegator: delegator, AgentTracer: tracer, SessionID: "sess-parallel", UserID: 42}

	raw := json.RawMessage(`{"tool_uses":[{"recipient_name":"functions.agent_call","tool_call_id":"child-agent-1","parameters":{"agent_name":"writer","prompt":"write a haiku"}}]}`)
	eng.dispatchTools(context.Background(), nil, []llm.ToolCall{{
		ID:   "parallel-1",
		Name: multitool.ToolName,
		Args: raw,
	}})

	if delegator.req.AgentName != "writer" {
		t.Fatalf("expected delegated agent writer, got %q", delegator.req.AgentName)
	}
	if delegator.req.ParentCallID != "child-agent-1" {
		t.Fatalf("expected child tool id as parent call id, got %q", delegator.req.ParentCallID)
	}
	if len(tracer.events) != 1 {
		t.Fatalf("expected one trace event, got %#v", tracer.events)
	}
	if tracer.events[0].ParentCallID != "child-agent-1" {
		t.Fatalf("expected traced parent call id child-agent-1, got %q", tracer.events[0].ParentCallID)
	}
}
