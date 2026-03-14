package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
)

type promptHandlerChatStore struct {
	sessions map[string]persistence.ChatSession
	messages map[string][]persistence.ChatMessage
}

func newPromptHandlerChatStore() *promptHandlerChatStore {
	return &promptHandlerChatStore{
		sessions: map[string]persistence.ChatSession{},
		messages: map[string][]persistence.ChatMessage{},
	}
}

func (s *promptHandlerChatStore) Init(context.Context) error { return nil }

func (s *promptHandlerChatStore) EnsureSession(_ context.Context, userID *int64, id string, name string) (persistence.ChatSession, error) {
	if sess, ok := s.sessions[id]; ok {
		return sess, nil
	}
	sess := persistence.ChatSession{ID: id, Name: name, UserID: userID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.sessions[id] = sess
	s.messages[id] = nil
	return sess, nil
}

func (s *promptHandlerChatStore) ListSessions(context.Context, *int64) ([]persistence.ChatSession, error) {
	out := make([]persistence.ChatSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out, nil
}

func (s *promptHandlerChatStore) GetSession(context.Context, *int64, string) (persistence.ChatSession, error) {
	return persistence.ChatSession{}, nil
}

func (s *promptHandlerChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	return s.EnsureSession(ctx, userID, name, name)
}

func (s *promptHandlerChatStore) RenameSession(_ context.Context, _ *int64, id, name string) (persistence.ChatSession, error) {
	sess := s.sessions[id]
	sess.Name = name
	s.sessions[id] = sess
	return sess, nil
}

func (s *promptHandlerChatStore) DeleteSession(context.Context, *int64, string) error { return nil }

func (s *promptHandlerChatStore) ListMessages(_ context.Context, _ *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	msgs := s.messages[sessionID]
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return append([]persistence.ChatMessage(nil), msgs...), nil
}

func (s *promptHandlerChatStore) DeleteMessage(context.Context, *int64, string, string) error {
	return nil
}

func (s *promptHandlerChatStore) DeleteMessagesAfter(context.Context, *int64, string, string, bool) error {
	return nil
}

func (s *promptHandlerChatStore) AppendMessages(_ context.Context, _ *int64, sessionID string, messages []persistence.ChatMessage, _ string, _ string) error {
	s.messages[sessionID] = append(s.messages[sessionID], messages...)
	return nil
}

func (s *promptHandlerChatStore) UpdateSummary(context.Context, *int64, string, string, int) error {
	return nil
}

func TestPromptHandlerRoutesSpecialistBeforeDevMockFallback(t *testing.T) {
	t.Parallel()

	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"specialist response","tool_calls":[]}}]}`))
	}))
	defer specialistServer.Close()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "orchestrator response"}}
	baseTools := tools.NewRegistry()
	cfg := config.Config{
		Workdir: ".",
		LLMClient: config.LLMClientConfig{
			Provider: "openai",
			OpenAI: config.OpenAIConfig{
				APIKey:  "test",
				BaseURL: specialistServer.URL,
				Model:   "spec-model",
			},
		},
	}
	specRegistry := specialists.NewRegistry(cfg.LLMClient, []config.SpecialistConfig{{
		Name:        "weather",
		Description: "Weather specialist",
		System:      "Respond as the weather specialist.",
		Model:       "spec-model",
	}}, specialistServer.Client(), baseTools)

	a := &app{
		cfg:              &cfg,
		llm:              baseProvider,
		baseToolRegistry: baseTools,
		specRegistry:     specRegistry,
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:             newRunStore(),
		engine: &agent.Engine{
			LLM:   baseProvider,
			Tools: baseTools,
			Model: "orchestrator-model",
		},
	}

	body := bytes.NewBufferString(`{"prompt":"forecast please","session_id":"sess-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/prompt?specialist=weather", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.promptHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := resp["result"]; got != "specialist response" {
		t.Fatalf("expected specialist response, got %q", got)
	}
	if len(a.runs.list()) != 1 {
		t.Fatalf("expected one recorded run, got %d", len(a.runs.list()))
	}
	if got := a.runs.list()[0].Status; got != "completed" {
		t.Fatalf("expected completed run, got %q", got)
	}
}

func TestPromptHandlerSystemPromptOverridesDirectSpecialistPrompt(t *testing.T) {
	t.Parallel()

	requestBodies := make(chan string, 1)
	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBodies <- string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"specialist response","tool_calls":[]}}]}`))
	}))
	defer specialistServer.Close()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "orchestrator response"}}
	baseTools := tools.NewRegistry()
	cfg := config.Config{
		Workdir: ".",
		LLMClient: config.LLMClientConfig{
			Provider: "openai",
			OpenAI: config.OpenAIConfig{
				APIKey:  "test",
				BaseURL: specialistServer.URL,
				Model:   "spec-model",
			},
		},
	}
	specRegistry := specialists.NewRegistry(cfg.LLMClient, []config.SpecialistConfig{{
		Name:        "gpt_bot",
		Description: "GPT specialist",
		System:      "Respond as the stored specialist prompt.",
		Model:       "spec-model",
	}}, specialistServer.Client(), baseTools)

	a := &app{
		cfg:              &cfg,
		llm:              baseProvider,
		baseToolRegistry: baseTools,
		specRegistry:     specRegistry,
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:             newRunStore(),
		engine: &agent.Engine{
			LLM:   baseProvider,
			Tools: baseTools,
			Model: "orchestrator-model",
		},
	}

	body := bytes.NewBufferString(`{"prompt":"hello","session_id":"sess-2","system_prompt":"You are the Matrix GPT bot. Reply only as yourself."}`)
	req := httptest.NewRequest(http.MethodPost, "/api/prompt?specialist=gpt_bot", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.promptHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	select {
	case raw := <-requestBodies:
		if !strings.Contains(raw, "You are the Matrix GPT bot. Reply only as yourself.") {
			t.Fatalf("expected override prompt in specialist request, got %s", raw)
		}
		if strings.Contains(raw, "Respond as the stored specialist prompt.") {
			t.Fatalf("expected stored specialist prompt to be overridden, got %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for specialist request")
	}
}

func TestPromptHandlerUsesSharedDevMockFallback(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/api/prompt", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	a.promptHandler().ServeHTTP(rr, req)

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
	if got := a.runs.list()[0].Status; got != "completed" {
		t.Fatalf("expected completed run, got %q", got)
	}
}

func TestHandleChatTarget_JSONIncludesQueuedMatrixMessages(t *testing.T) {
	t.Parallel()

	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"specialist response","tool_calls":[]}}]}`))
	}))
	defer specialistServer.Close()

	a := newSpecialistTestApp(t, specialistServer.URL, []config.SpecialistConfig{{
		Name:        "weather",
		Description: "Weather specialist",
		System:      "Respond as the weather specialist.",
		Model:       "spec-model",
	}})

	outbox := sandbox.NewMatrixOutbox()
	outbox.Add("!room:test", "Pulse update")
	ctx := sandbox.WithMatrixOutbox(sandbox.WithRoomID(context.Background(), "!room:test"), outbox)
	req := httptest.NewRequest(http.MethodPost, "/api/prompt?specialist=weather", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	handled := a.handleChatTarget(rr, req, chatDispatchTarget{SpecialistName: "weather"}, "forecast please", "sess-json", "", nil, nil, 0, chatTargetDescriptor{})
	if !handled {
		t.Fatalf("expected specialist handler to process request")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Result         string                  `json:"result"`
		MatrixMessages []sandbox.MatrixMessage `json:"matrix_messages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Result != "specialist response" {
		t.Fatalf("expected specialist response, got %q", resp.Result)
	}
	if len(resp.MatrixMessages) != 1 || resp.MatrixMessages[0].RoomID != "!room:test" || resp.MatrixMessages[0].Text != "Pulse update" {
		t.Fatalf("unexpected matrix messages: %#v", resp.MatrixMessages)
	}
}

func TestHandleChatTarget_SSEIncludesQueuedMatrixMessages(t *testing.T) {
	t.Parallel()

	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"specialist response\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n"))
	}))
	defer specialistServer.Close()

	a := newSpecialistTestApp(t, specialistServer.URL, []config.SpecialistConfig{{
		Name:        "weather",
		Description: "Weather specialist",
		System:      "Respond as the weather specialist.",
		Model:       "spec-model",
	}})

	outbox := sandbox.NewMatrixOutbox()
	outbox.Add("!room:test", "Pulse update")
	ctx := sandbox.WithMatrixOutbox(sandbox.WithRoomID(context.Background(), "!room:test"), outbox)
	req := httptest.NewRequest(http.MethodPost, "/api/prompt?specialist=weather", nil).WithContext(ctx)
	req.Header.Set("Accept", "text/event-stream")
	rr := httptest.NewRecorder()

	handled := a.handleChatTarget(rr, req, chatDispatchTarget{SpecialistName: "weather"}, "forecast please", "sess-sse", "", nil, nil, 0, chatTargetDescriptor{})
	if !handled {
		t.Fatalf("expected specialist handler to process request")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "\"type\":\"final\"") {
		t.Fatalf("expected final SSE event, got %s", body)
	}
	if !strings.Contains(body, "\"matrix_messages\":[{") || !strings.Contains(body, "Pulse update") || !strings.Contains(body, "!room:test") {
		t.Fatalf("expected queued matrix messages in SSE payload, got %s", body)
	}
}

func newSpecialistTestApp(t *testing.T, baseURL string, specs []config.SpecialistConfig) *app {
	t.Helper()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "orchestrator response"}}
	baseTools := tools.NewRegistry()
	cfg := config.Config{
		Workdir:  ".",
		MaxSteps: 2,
		LLMClient: config.LLMClientConfig{
			Provider: "openai",
			OpenAI: config.OpenAIConfig{
				APIKey:  "test",
				BaseURL: baseURL,
				Model:   "spec-model",
			},
		},
	}

	return &app{
		cfg:              &cfg,
		llm:              baseProvider,
		baseToolRegistry: baseTools,
		specRegistry:     specialists.NewRegistry(cfg.LLMClient, specs, http.DefaultClient, baseTools),
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:             newRunStore(),
		engine: &agent.Engine{
			LLM:   baseProvider,
			Tools: baseTools,
			Model: "orchestrator-model",
		},
	}
}
