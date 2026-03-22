package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
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

func TestAgentRunHandlerDeletesEphemeralSessionAfterSuccess(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "temporary response"}}
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

	body := bytes.NewBufferString(`{"prompt":"hello","session_id":"ephemeral-sess","ephemeral_session":true}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.agentRunHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if _, exists := chatStore.sessions["ephemeral-sess"]; exists {
		t.Fatalf("expected ephemeral session to be removed after success")
	}
	if msgs := chatStore.messages["ephemeral-sess"]; len(msgs) != 0 {
		t.Fatalf("expected ephemeral session messages to be removed, got %d", len(msgs))
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
