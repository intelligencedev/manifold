package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/persistence"
	"manifold/internal/specialists"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
)

func TestDispatchBuiltChatTargetMapsNotFoundBuildError(t *testing.T) {
	t.Parallel()

	a := &app{}
	req := httptest.NewRequest(http.MethodPost, "/agent/run", nil)
	rec := httptest.NewRecorder()

	handled := a.dispatchBuiltChatTarget(rec, req, chatTargetDispatchOptions{
		Prompt:    "hello",
		SessionID: "default",
		Build: func(context.Context) chatEngineBuildResult {
			return chatEngineBuildResult{StatusCode: http.StatusNotFound, Err: context.Canceled}
		},
		NotFoundMessage:      "specialist not found",
		InternalErrorMessage: "specialist registry unavailable",
	})

	if !handled {
		t.Fatal("expected request to be handled")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "specialist not found\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestDispatchBuiltChatTargetMapsInternalBuildError(t *testing.T) {
	t.Parallel()

	a := &app{}
	req := httptest.NewRequest(http.MethodPost, "/agent/run", nil)
	rec := httptest.NewRecorder()

	handled := a.dispatchBuiltChatTarget(rec, req, chatTargetDispatchOptions{
		Prompt:    "hello",
		SessionID: "default",
		Build: func(context.Context) chatEngineBuildResult {
			return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: context.Canceled}
		},
		NotFoundMessage:      "team not found",
		InternalErrorMessage: "failed to load team",
	})

	if !handled {
		t.Fatal("expected request to be handled")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "failed to load team\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestDescribeChatTargetPrefersSpecialistOverTeam(t *testing.T) {
	t.Parallel()

	a := &app{}
	descriptor, ok := a.describeChatTarget(chatDispatchTarget{SpecialistName: "weather", TeamName: "ops"}, "override", 7)
	if !ok {
		t.Fatal("expected target descriptor")
	}
	if descriptor.NotFoundMessage != "specialist not found" {
		t.Fatalf("unexpected not found message: %q", descriptor.NotFoundMessage)
	}
	if !descriptor.Stream.IncludeMatrixMessages {
		t.Fatal("expected specialist stream to include matrix messages")
	}
	if !descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected specialist JSON to include matrix messages")
	}
	if descriptor.Build == nil {
		t.Fatal("expected build function")
	}
}

func TestDescribeChatTargetSkipsOrchestratorSpecialistForTeam(t *testing.T) {
	t.Parallel()

	a := &app{cfg: &config.Config{WorkflowTimeoutSeconds: 90, AgentRunTimeoutSeconds: 30}}
	descriptor, ok := a.describeChatTarget(chatDispatchTarget{SpecialistName: specialists.OrchestratorName, TeamName: "ops"}, "", 7)
	if !ok {
		t.Fatal("expected team target descriptor")
	}
	if descriptor.NotFoundMessage != "team not found" {
		t.Fatalf("unexpected not found message: %q", descriptor.NotFoundMessage)
	}
	if descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected team JSON matrix messages to remain disabled")
	}
	if descriptor.JSON.TimeoutSeconds != 90 {
		t.Fatalf("expected workflow timeout for team JSON, got %d", descriptor.JSON.TimeoutSeconds)
	}
	if descriptor.Stream.TimeoutSeconds != 90 {
		t.Fatalf("expected workflow timeout for team stream, got %d", descriptor.Stream.TimeoutSeconds)
	}
}

func TestAgentRunOrchestratorDescriptorRequestsSummary(t *testing.T) {
	t.Parallel()

	a := &app{}
	descriptor := a.agentRunOrchestratorDescriptor(context.Background(), 7, chatRunRequest{Prompt: "hello", SessionID: "sess-1"}, nil)
	if descriptor.InternalErrorMessage != "agent unavailable" {
		t.Fatalf("unexpected internal error message: %q", descriptor.InternalErrorMessage)
	}
	if !descriptor.IncludeSummary {
		t.Fatal("expected agent run orchestrator descriptor to request summary events")
	}
	if descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected agent run orchestrator JSON to omit matrix messages")
	}
}

func TestDispatchOptionsFromDescriptorCarriesIncludeSummary(t *testing.T) {
	t.Parallel()

	userID := int64(7)
	opts := dispatchOptionsFromDescriptor(chatTargetDescriptor{IncludeSummary: true}, "hello", "sess-1", false, &userID)
	if !opts.IncludeSummary {
		t.Fatal("expected include summary flag to be preserved")
	}
}

func TestDispatchBuiltChatTargetUsesBuiltProviderForHistory(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "ignored"}}
	if _, err := chatStore.EnsureSession(context.Background(), nil, "sess-provider", "sess-provider"); err != nil {
		t.Fatalf("ensure session: %v", err)
	}
	if err := chatStore.AppendMessages(context.Background(), nil, "sess-provider", []persistence.ChatMessage{
		{Role: "user", Content: "earlier turn"},
		{Role: "assistant", Content: "stored reply"},
	}, "stored reply", "test-model"); err != nil {
		t.Fatalf("append messages: %v", err)
	}
	if err := chatStore.UpdateSummary(context.Background(), nil, "sess-provider", `{"type":"compaction","encrypted_content":"opaque"}`, 2); err != nil {
		t.Fatalf("update summary: %v", err)
	}

	provider := &recordingProvider{resp: llm.Message{Role: "assistant", Content: "ok"}}
	a := &app{
		cfg:        &config.Config{Workdir: "."},
		chatStore:  chatStore,
		chatMemory: memory.NewManager(chatStore, baseProvider, memory.Config{}),
		runs:       newRunStore(),
	}
	req := httptest.NewRequest(http.MethodPost, "/agent/run", nil)
	rec := httptest.NewRecorder()

	handled := a.dispatchBuiltChatTarget(rec, req, chatTargetDispatchOptions{
		Prompt:    "hello",
		SessionID: "sess-provider",
		Build: func(context.Context) chatEngineBuildResult {
			return chatEngineBuildResult{
				Engine:     &agent.Engine{LLM: provider, Tools: tools.NewRegistry(), MaxSteps: 1},
				ModelLabel: "test-model",
			}
		},
	})

	if !handled {
		t.Fatal("expected request to be handled")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("expected provider to receive messages")
	}
	for _, msg := range provider.lastMessages {
		if msg.Compaction != nil {
			t.Fatalf("expected non-compaction provider history, got compaction message: %#v", msg)
		}
		if msg.Content == "When continuing from prior context (including compacted context), do not restate prior final answers unless the user asks. Only provide new information, the next steps, or the requested delta." {
			t.Fatal("expected compaction continuation rule to be omitted for non-compaction provider")
		}
	}
}

type recordingProvider struct {
	resp         llm.Message
	lastMessages []llm.Message
}

func (p *recordingProvider) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string) (llm.Message, error) {
	p.lastMessages = append([]llm.Message(nil), msgs...)
	return p.resp, nil
}

func (p *recordingProvider) ChatStream(_ context.Context, msgs []llm.Message, _ []llm.ToolSchema, _ string, h llm.StreamHandler) error {
	p.lastMessages = append([]llm.Message(nil), msgs...)
	h.OnDelta(p.resp.Content)
	return nil
}

func TestPromptOrchestratorDescriptorIncludesMatrixMessages(t *testing.T) {
	t.Parallel()

	a := &app{}
	descriptor := a.promptOrchestratorDescriptor(context.Background(), 7, chatRunRequest{Prompt: "hello", SessionID: "sess-1", SystemPrompt: "override"}, nil)
	if !descriptor.JSON.IncludeMatrixMessages {
		t.Fatal("expected prompt orchestrator JSON to include matrix messages")
	}
	if descriptor.Stream.StructuredErrors {
		t.Fatal("expected prompt orchestrator stream errors to remain unstructured")
	}
	if descriptor.RunContext == nil {
		t.Fatal("expected prompt orchestrator run context")
	}
	_ = llm.WithUserID(context.Background(), 1)
}
