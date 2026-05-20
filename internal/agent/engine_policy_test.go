package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"manifold/internal/agent/memory"
	"manifold/internal/llm"
	"manifold/internal/policy"
	"manifold/internal/tools"
)

type recordingTool struct {
	called bool
}

func TestRunWithReMemInjectsSoftPolicyContext(t *testing.T) {
	t.Parallel()

	provider := &captureMessageProvider{}
	em := memory.NewEvolvingMemory(memory.EvolvingMemoryConfig{
		LLM: provider,
	})
	eng := &Engine{
		LLM:            provider,
		Tools:          tools.NewRegistry(),
		MaxSteps:       1,
		System:         "base system",
		UserID:         7,
		ProjectID:      "project-1",
		AgentRole:      "orchestrator",
		EvolvingMemory: em,
		ReMemEnabled:   true,
		ReMemController: memory.NewReMemController(memory.ReMemConfig{
			LLM:           provider,
			Memory:        em,
			MaxInnerSteps: 1,
		}),
		PolicyEnforcer: policy.NewStaticEnforcer([]policy.Record{{
			ID:            "prefer-review",
			Scope:         policy.ScopeProject,
			TenantID:      7,
			ProjectID:     "project-1",
			Severity:      policy.SeveritySoft,
			Statement:     "Prefer reviewer confirmation before deployment.",
			ApprovalState: policy.ApprovalApproved,
			Targets:       policy.TargetSelector{Roles: []string{"orchestrator"}},
		}}),
	}

	if _, _, err := eng.runWithReMem(context.Background(), "deploy", nil); err != nil {
		t.Fatalf("runWithReMem returned error: %v", err)
	}
	if len(provider.messages) < 2 || provider.messages[1].Role != "user" || !strings.Contains(provider.messages[1].Content, "## Runtime Policy Context") || !strings.Contains(provider.messages[1].Content, "Prefer reviewer confirmation") {
		t.Fatalf("expected soft policy context in ReMem user prompt, got %#v", provider.messages)
	}
}

func (t *recordingTool) Name() string { return "write_file" }
func (t *recordingTool) JSONSchema() map[string]any {
	return map[string]any{"description": "test tool", "parameters": map[string]any{"type": "object"}}
}
func (t *recordingTool) Call(context.Context, json.RawMessage) (any, error) {
	t.called = true
	return map[string]any{"ok": true}, nil
}

func TestExecuteToolCallBlocksHardPolicy(t *testing.T) {
	t.Parallel()

	tool := &recordingTool{}
	registry := tools.NewRegistry()
	registry.Register(tool)
	eng := &Engine{
		Tools:     registry,
		UserID:    7,
		ProjectID: "project-1",
		AgentRole: "orchestrator",
		PolicyEnforcer: policy.NewStaticEnforcer([]policy.Record{{
			ID:            "no-secret-write",
			Scope:         policy.ScopeProject,
			TenantID:      7,
			ProjectID:     "project-1",
			Severity:      policy.SeverityHard,
			Statement:     "Do not write under secrets.",
			ApprovalState: policy.ApprovalApproved,
			Targets:       policy.TargetSelector{Tools: []string{"write_file"}, PathPrefixes: []string{"secrets"}},
		}}),
	}

	msg := eng.executeToolCall(context.Background(), llm.ToolCall{Name: "write_file", ID: "tool-1", Args: json.RawMessage(`{"path":"secrets/token.txt"}`)})
	if tool.called {
		t.Fatal("expected hard policy to block before tool dispatch")
	}
	if !strings.Contains(msg.Content, "tool call blocked by policy") || !strings.Contains(msg.Content, "no-secret-write") {
		t.Fatalf("unexpected block payload %q", msg.Content)
	}
}

func TestRunInjectsSoftPolicyContext(t *testing.T) {
	t.Parallel()

	provider := &captureMessageProvider{}
	eng := &Engine{
		LLM:       provider,
		Tools:     tools.NewRegistry(),
		MaxSteps:  1,
		System:    "base system",
		UserID:    7,
		ProjectID: "project-1",
		AgentRole: "orchestrator",
		PolicyEnforcer: policy.NewStaticEnforcer([]policy.Record{{
			ID:            "prefer-review",
			Scope:         policy.ScopeProject,
			TenantID:      7,
			ProjectID:     "project-1",
			Severity:      policy.SeveritySoft,
			Statement:     "Prefer reviewer confirmation before deployment.",
			ApprovalState: policy.ApprovalApproved,
			Targets:       policy.TargetSelector{Roles: []string{"orchestrator"}},
		}}),
	}

	if _, err := eng.Run(context.Background(), "deploy", nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(provider.messages) < 2 || provider.messages[1].Role != "user" || !strings.Contains(provider.messages[1].Content, "## Runtime Policy Context") || !strings.Contains(provider.messages[1].Content, "Prefer reviewer confirmation") {
		t.Fatalf("expected soft policy context in user prompt, got %#v", provider.messages)
	}
	if strings.Contains(provider.messages[0].Content, "## Runtime Policy Context") {
		t.Fatalf("did not expect soft policy context in system prompt, got %#v", provider.messages)
	}
}
