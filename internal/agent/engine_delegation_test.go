package agent

import (
	"context"
	"encoding/json"
	"testing"

	"manifold/internal/llm"
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
	eng := &Engine{Delegator: spy, SessionID: "sess-delegate"}
	args, err := json.Marshal(map[string]any{
		"agent_name": "writer",
		"prompt":     "draft this",
		"user_id":    7,
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
	if spy.req.AgentName != "writer" {
		t.Fatalf("expected agent writer, got %q", spy.req.AgentName)
	}
	if string(payload) == "" {
		t.Fatal("expected payload from delegated run")
	}
}
