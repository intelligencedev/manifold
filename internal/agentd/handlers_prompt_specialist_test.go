package agentd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/projects"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
)

type promptHandlerChatStore struct {
	mu       sync.RWMutex
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
	return s.EnsureSessionKind(context.Background(), userID, id, name, persistence.ChatSessionKindChat)
}

func (s *promptHandlerChatStore) EnsureSessionKind(_ context.Context, userID *int64, id string, name string, kind string) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		return sess, nil
	}
	if strings.TrimSpace(kind) == "" {
		kind = persistence.ChatSessionKindChat
	}
	sess := persistence.ChatSession{ID: id, Name: name, Kind: kind, UserID: userID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.sessions[id] = sess
	s.messages[id] = nil
	return sess, nil
}

func (s *promptHandlerChatStore) ListSessions(context.Context, *int64) ([]persistence.ChatSession, error) {
	return s.ListSessionsByKind(context.Background(), nil, persistence.ChatSessionKindChat)
}

func (s *promptHandlerChatStore) ListSessionsByKind(_ context.Context, _ *int64, kind string) ([]persistence.ChatSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(kind) == "" {
		kind = persistence.ChatSessionKindChat
	}
	out := make([]persistence.ChatSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessKind := strings.TrimSpace(sess.Kind)
		if sessKind == "" {
			sessKind = persistence.ChatSessionKindChat
		}
		if sessKind != kind {
			continue
		}
		out = append(out, sess)
	}
	return out, nil
}

func (s *promptHandlerChatStore) GetSession(_ context.Context, _ *int64, id string) (persistence.ChatSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	return sess, nil
}

func (s *promptHandlerChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	return s.CreateSessionKind(ctx, userID, name, persistence.ChatSessionKindChat)
}

func (s *promptHandlerChatStore) CreateSessionKind(ctx context.Context, userID *int64, name string, kind string) (persistence.ChatSession, error) {
	return s.EnsureSessionKind(ctx, userID, name, name, kind)
}

func (s *promptHandlerChatStore) RenameSession(_ context.Context, _ *int64, id, name string) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[id]
	sess.Name = name
	s.sessions[id] = sess
	return sess, nil
}

func (s *promptHandlerChatStore) DeleteSession(_ context.Context, _ *int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	delete(s.messages, id)
	return nil
}

func (s *promptHandlerChatStore) ListMessages(_ context.Context, _ *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[sessionID] = append(s.messages[sessionID], messages...)
	return nil
}

func (s *promptHandlerChatStore) UpdateSummary(_ context.Context, _ *int64, sessionID string, summary string, summarizedCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return persistence.ErrNotFound
	}
	sess.Summary = summary
	sess.SummarizedCount = summarizedCount
	s.sessions[sessionID] = sess
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

	handled := a.handleChatTarget(rr, req, chatDispatchTarget{SpecialistName: "weather"}, "forecast please", "sess-json", "", "", false, "", nil, 0, chatTargetDescriptor{})
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

func TestHandleChatTargetUsesImageAPIForImageGenerationSpecialist(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]any
	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/images/generations" {
			http.Error(w, "unexpected chat request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"cG5nYnl0ZXM="}]}`))
	}))
	defer specialistServer.Close()

	a := newSpecialistTestApp(t, specialistServer.URL, []config.SpecialistConfig{{
		Name:            "image-maker",
		Description:     "Image generation specialist",
		Provider:        "openai",
		BaseURL:         specialistServer.URL,
		APIKey:          "test",
		Model:           "gpt-image-2",
		System:          "Never send this system prompt to image generation.",
		EnableTools:     true,
		ImageGeneration: true,
		ExtraParams:     map[string]any{"size": "2048x2048"},
	}})
	store := a.chatStore.(*promptHandlerChatStore)
	store.messages["sess-image"] = []persistence.ChatMessage{
		{Role: "user", Content: "previous prompt that must be ignored"},
		{Role: "assistant", Content: "previous answer that must be ignored"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/prompt?specialist=image-maker", nil)
	rr := httptest.NewRecorder()

	handled := a.handleChatTarget(rr, req, chatDispatchTarget{SpecialistName: "image-maker"}, "draw a river", "sess-image", "", "", false, "", nil, 0, chatTargetDescriptor{})
	if !handled {
		t.Fatalf("expected specialist handler to process request")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotPath != "/images/generations" {
		t.Fatalf("expected image generation endpoint, got %q", gotPath)
	}
	if model, _ := gotBody["model"].(string); model != "gpt-image-2" {
		t.Fatalf("expected gpt-image-2 model, got %#v", gotBody["model"])
	}
	if prompt, _ := gotBody["prompt"].(string); prompt != "draw a river" {
		t.Fatalf("expected prompt forwarded to image API, got %#v", gotBody["prompt"])
	}
	if size, _ := gotBody["size"].(string); size != "2048x2048" {
		t.Fatalf("expected configured image size, got %#v", gotBody["size"])
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

	handled := a.handleChatTarget(rr, req, chatDispatchTarget{SpecialistName: "weather"}, "forecast please", "sess-sse", "", "", false, "", nil, 0, chatTargetDescriptor{})
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
	workdir := t.TempDir()
	cfg := config.Config{
		Workdir:  workdir,
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
	projectsService := projects.NewService(workdir, "")

	return &app{
		cfg:                &cfg,
		llm:                baseProvider,
		baseToolRegistry:   baseTools,
		specRegistry:       specialists.NewRegistry(cfg.LLMClient, specs, http.DefaultClient, baseTools),
		chatStore:          chatStore,
		matrixMessageStore: databases.NewMatrixMessageStore(nil),
		chatMemory:         memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:               newRunStore(),
		projectsService:    projectsService,
		workspaceManager:   workspaces.NewManager(&cfg),
		engine: &agent.Engine{
			LLM:   baseProvider,
			Tools: baseTools,
			Model: "orchestrator-model",
		},
	}
}
