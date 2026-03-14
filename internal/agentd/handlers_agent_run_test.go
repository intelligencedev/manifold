package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
	"manifold/internal/warpp"
)

func TestAgentRunHandlerOrchestratorFallbackRecordsSingleRun(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "orchestrator response"}}
	baseTools := tools.NewRegistry()
	a := &app{
		cfg: &config.Config{
			Workdir:  ".",
			MaxSteps: 2,
			OpenAI: config.OpenAIConfig{
				APIKey: "test",
				Model:  "orchestrator-model",
			},
			LLMClient: config.LLMClientConfig{
				Provider: "openai",
				OpenAI: config.OpenAIConfig{
					APIKey: "test",
					Model:  "orchestrator-model",
				},
			},
		},
		llm:              baseProvider,
		baseToolRegistry: baseTools,
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:             newRunStore(),
		engine: &agent.Engine{
			LLM:      baseProvider,
			Tools:    baseTools,
			Model:    "orchestrator-model",
			MaxSteps: 2,
		},
	}

	body := bytes.NewBufferString(`{"prompt":"hello","session_id":"sess-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.agentRunHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := resp["result"]; got != "orchestrator response" {
		t.Fatalf("expected orchestrator response, got %q", got)
	}
	if len(a.runs.list()) != 1 {
		t.Fatalf("expected one recorded run, got %d", len(a.runs.list()))
	}
	if got := a.runs.list()[0].Status; got != "completed" {
		t.Fatalf("expected completed run, got %q", got)
	}
}

func TestAgentRunHandlerWarppUsesExtractedWorkflowPath(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "orchestrator response"}}
	baseTools := tools.NewRegistry()
	baseTools.Register(agentRunFunctionalTool{name: "echo_step", call: func(ctx context.Context, raw json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	}})
	workflow := warpp.Workflow{
		Intent:   "echo_intent",
		Keywords: []string{"echo"},
		Steps: []warpp.Step{
			{ID: "s1", Text: "echo", Tool: &warpp.ToolRef{Name: "echo_step", Args: map[string]any{"text": "${A.utter}"}}},
		},
	}
	a := &app{
		cfg: &config.Config{
			Workdir:  ".",
			MaxSteps: 2,
			OpenAI: config.OpenAIConfig{
				APIKey: "test",
				Model:  "orchestrator-model",
			},
			LLMClient: config.LLMClientConfig{
				Provider: "openai",
				OpenAI: config.OpenAIConfig{
					APIKey: "test",
					Model:  "orchestrator-model",
				},
			},
		},
		llm:              baseProvider,
		baseToolRegistry: baseTools,
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:             newRunStore(),
		engine: &agent.Engine{
			LLM:      baseProvider,
			Tools:    baseTools,
			Model:    "orchestrator-model",
			MaxSteps: 2,
		},
		warppRunner:     &warpp.Runner{Tools: baseTools},
		warppRegistries: map[int64]*warpp.Registry{systemUserID: {}},
	}
	a.warppRegistries[systemUserID].Upsert(workflow, "")

	body := bytes.NewBufferString(`{"prompt":"please echo this","session_id":"sess-warpp"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run?warpp=true", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.agentRunHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := resp["result"]; !strings.Contains(got, "WARPP: executing intent echo_intent") {
		t.Fatalf("expected WARPP summary, got %q", got)
	}
	if got := resp["result"]; !strings.Contains(got, "- echo") {
		t.Fatalf("expected workflow step summary, got %q", got)
	}
	if len(a.runs.list()) != 0 {
		t.Fatalf("expected warpp path to avoid chat run records, got %d", len(a.runs.list()))
	}
}

func TestAgentRunHandlerUsesSharedDevMockFallback(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "orchestrator response"}}
	a := &app{
		cfg:              &config.Config{},
		llm:              baseProvider,
		baseToolRegistry: tools.NewRegistry(),
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:             newRunStore(),
	}

	body := bytes.NewBufferString(`{"prompt":"hello","session_id":"sess-dev"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.agentRunHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := resp["result"]; got != "(dev) mock response: hello" {
		t.Fatalf("expected dev mock response, got %q", got)
	}
	if len(a.runs.list()) != 1 {
		t.Fatalf("expected one recorded run, got %d", len(a.runs.list()))
	}
}

type agentRunFunctionalTool struct {
	name string
	call func(context.Context, json.RawMessage) (any, error)
}

func (t agentRunFunctionalTool) Name() string { return t.name }
func (t agentRunFunctionalTool) JSONSchema() map[string]any {
	return map[string]any{"description": "test tool"}
}
func (t agentRunFunctionalTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return t.call(ctx, raw)
}
