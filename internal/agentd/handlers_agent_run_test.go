package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
)

type agentRunScriptedProvider struct {
	responses       []llm.Message
	streamResponses []agentRunStreamResponse
}

type agentRunStreamResponse struct {
	Deltas    []string
	ToolCalls []llm.ToolCall
	Err       error
}

func (p *agentRunScriptedProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	if len(p.responses) == 0 {
		return llm.Message{}, errors.New("no scripted response")
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

func (p *agentRunScriptedProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, h llm.StreamHandler) error {
	if len(p.streamResponses) == 0 {
		return errors.New("no scripted stream response")
	}
	resp := p.streamResponses[0]
	p.streamResponses = p.streamResponses[1:]
	if resp.Err != nil {
		return resp.Err
	}
	for _, delta := range resp.Deltas {
		h.OnDelta(delta)
	}
	for _, call := range resp.ToolCalls {
		h.OnToolCall(call)
	}
	return nil
}

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
	sessionID := normalizeClientChatSessionID("ephemeral-sess")
	if _, exists := chatStore.sessions[sessionID]; exists {
		t.Fatalf("expected ephemeral session to be removed after success")
	}
	if msgs := chatStore.messages[sessionID]; len(msgs) != 0 {
		t.Fatalf("expected ephemeral session messages to be removed, got %d", len(msgs))
	}
}

func TestAgentRunHandlerHarnessWorkflowPersistsTurn(t *testing.T) {
	t.Parallel()

	searchCalls := 0
	provider := &agentRunScriptedProvider{responses: []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-search", Name: "lookup", Args: json.RawMessage(`{"query":"forge"}`)}}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-final", Name: "agent_response", Args: json.RawMessage(`{"text":"workflow complete"}`)}}},
	}}
	baseTools := tools.NewRegistry()
	baseTools.Register(agentRunFunctionalTool{
		name: "lookup",
		call: func(context.Context, json.RawMessage) (any, error) {
			searchCalls++
			return map[string]any{"ok": true, "result": "found"}, nil
		},
	})
	chatStore := newPromptHandlerChatStore()
	a := newHarnessAgentRunTestApp(provider, baseTools, chatStore)

	body := bytes.NewBufferString(`{"prompt":"run workflow","session_id":"forge-json"}`)
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
	if got := resp["result"]; got != "workflow complete" {
		t.Fatalf("expected workflow result, got %q", got)
	}
	if searchCalls != 1 {
		t.Fatalf("expected lookup to run once, got %d", searchCalls)
	}
	sessionID := normalizeClientChatSessionID("forge-json")
	messages := chatStore.messages[sessionID]
	if len(messages) != 5 {
		t.Fatalf("expected user plus four turn messages, got %d: %+v", len(messages), messages)
	}
	if messages[0].Role != "user" || messages[1].Role != "assistant" || messages[2].Role != "tool" || messages[3].Role != "assistant" || messages[4].Role != "tool" {
		t.Fatalf("unexpected persisted roles: %+v", messages)
	}
	if !strings.Contains(messages[1].Content, "lookup") || !strings.Contains(messages[3].Content, "agent_response") {
		t.Fatalf("expected assistant tool calls to persist, got %+v", messages)
	}
	if len(a.runs.list()) != 1 || a.runs.list()[0].Status != "completed" {
		t.Fatalf("expected completed run, got %+v", a.runs.list())
	}
}

func TestAgentRunHandlerHarnessWorkflowSSEEventsRemainCompatible(t *testing.T) {
	t.Parallel()

	provider := &agentRunScriptedProvider{streamResponses: []agentRunStreamResponse{
		{ToolCalls: []llm.ToolCall{{ID: "call-search", Name: "lookup", Args: json.RawMessage(`{"query":"forge"}`)}}},
		{ToolCalls: []llm.ToolCall{{ID: "call-final", Name: "agent_response", Args: json.RawMessage(`{"text":"stream complete"}`)}}},
	}}
	baseTools := tools.NewRegistry()
	baseTools.Register(agentRunFunctionalTool{
		name: "lookup",
		call: func(context.Context, json.RawMessage) (any, error) {
			return map[string]any{"ok": true, "result": "found"}, nil
		},
	})
	chatStore := newPromptHandlerChatStore()
	a := newHarnessAgentRunTestApp(provider, baseTools, chatStore)

	body := bytes.NewBufferString(`{"prompt":"run workflow","session_id":"forge-stream"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	rr := httptest.NewRecorder()

	a.agentRunHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %q", got)
	}
	bodyText := rr.Body.String()
	for _, want := range []string{`"type":"tool_start"`, `"type":"tool_result"`, `"type":"final"`, `"data":"stream complete"`} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("expected SSE body to contain %s, got %s", want, bodyText)
		}
	}
	sessionID := normalizeClientChatSessionID("forge-stream")
	if messages := chatStore.messages[sessionID]; len(messages) != 5 {
		t.Fatalf("expected persisted streaming turn messages, got %d: %+v", len(messages), messages)
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

func newHarnessAgentRunTestApp(provider llm.Provider, baseTools tools.Registry, chatStore *promptHandlerChatStore) *app {
	cfg := &config.Config{
		Workdir:  ".",
		MaxSteps: 4,
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
		Harness: config.HarnessConfig{
			Enabled:           true,
			Mode:              "workflow",
			MaxRetriesPerStep: 3,
			MaxToolErrors:     2,
			TerminalTools:     []string{"agent_response"},
			RequiredSteps:     []string{"lookup"},
		},
	}
	return &app{
		cfg:              cfg,
		llm:              provider,
		baseToolRegistry: baseTools,
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, provider, memory.Config{}),
		runs:             newRunStore(),
		engine: &agent.Engine{
			LLM:            provider,
			Tools:          baseTools,
			Model:          "orchestrator-model",
			MaxSteps:       cfg.MaxSteps,
			HarnessEnabled: cfg.Harness.Enabled,
			HarnessConfig:  harnessRunConfig(cfg.Harness),
		},
	}
}
