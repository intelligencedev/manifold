package agentd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"manifold/internal/agent"
	"manifold/internal/durable"
	"manifold/internal/llm"
	"manifold/internal/persistence/databases"
)

type llmCaptureBoundaryProvider struct{}

func (p *llmCaptureBoundaryProvider) Chat(context.Context, []llm.Message, []llm.ToolSchema, string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: "ok"}, nil
}

func (p *llmCaptureBoundaryProvider) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ string, h llm.StreamHandler) error {
	if h != nil {
		h.OnDelta("ok")
	}
	return nil
}

func TestAttachLLMRequestCaptureStoresProviderBoundaryMessages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-1", "Session"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	requestStore := databases.NewMemoryLLMRequestStore(chatStore)
	eng := &agent.Engine{
		LLM:       &llmCaptureBoundaryProvider{},
		Model:     "gpt-test",
		AgentRole: "fable",
	}

	attachLLMRequestCapture(eng, llmRequestCaptureConfig{
		Store:               requestStore,
		SessionID:           "sess-1",
		RunID:               "run-1",
		MessageID:           "assistant-1",
		ParentUserMessageID: "user-1",
		SpecialistID:        "fable",
	})

	messages := []llm.Message{
		{Role: "system", Content: "final system context"},
		{Role: "user", Content: "final user prompt after memory injection"},
	}
	tools := []llm.ToolSchema{{
		Name:        "read_file",
		Description: "Read a file",
		Parameters:  map[string]any{"type": "object"},
	}}
	if _, err := eng.LLM.Chat(ctx, messages, tools, "gpt-test"); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	requests, err := requestStore.ListLLMRequestsForMessage(ctx, nil, "sess-1", "assistant-1")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one captured request, got %d", len(requests))
	}
	if requests[0].MessageID != "assistant-1" || requests[0].ParentUserMessageID != "user-1" {
		t.Fatalf("expected message associations, got message=%q parent=%q", requests[0].MessageID, requests[0].ParentUserMessageID)
	}
	if requests[0].SpecialistID != "fable" || requests[0].Model != "gpt-test" {
		t.Fatalf("expected metadata from capture config/provider call, got specialist=%q model=%q", requests[0].SpecialistID, requests[0].Model)
	}

	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(requests[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Messages) != 2 {
		t.Fatalf("expected exact sent message array, got %#v", payload.Messages)
	}
	if payload.Messages[0].Role != "system" || payload.Messages[0].Content != "final system context" {
		t.Fatalf("expected system message captured, got %#v", payload.Messages[0])
	}
	if payload.Messages[1].Role != "user" || payload.Messages[1].Content != "final user prompt after memory injection" {
		t.Fatalf("expected user message captured, got %#v", payload.Messages[1])
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "read_file" {
		t.Fatalf("expected tool schema captured, got %#v", payload.Tools)
	}
}

func TestAttachLLMRequestCaptureDoesNotDuplicateEngineSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-1", "Session"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	requestStore := databases.NewMemoryLLMRequestStore(chatStore)
	eng := &agent.Engine{
		LLM:       &llmCaptureBoundaryProvider{},
		Model:     "gpt-test",
		AgentRole: "fable",
	}

	attachLLMRequestCapture(eng, llmRequestCaptureConfig{
		Store:               requestStore,
		SessionID:           "sess-1",
		RunID:               "run-1",
		MessageID:           "assistant-1",
		ParentUserMessageID: "user-1",
		SpecialistID:        "fable",
	})

	messages := []llm.Message{{Role: "user", Content: "same provider call"}}
	tools := []llm.ToolSchema{{Name: "read_file"}}
	eng.OnLLMRequest(agent.LLMRequestSnapshot{
		ID:        "engine-req-1",
		Messages:  messages,
		Tools:     tools,
		Model:     "gpt-test",
		CreatedAt: time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
	})
	if _, err := eng.LLM.Chat(ctx, messages, tools, "gpt-test"); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	requests, err := requestStore.ListLLMRequestsForMessage(ctx, nil, "sess-1", "assistant-1")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one de-duplicated request, got %d", len(requests))
	}
	if requests[0].ID != "engine-req-1" {
		t.Fatalf("expected engine request ID to remain inspectable, got %q", requests[0].ID)
	}
}

func TestAttachLLMRequestCaptureReplacesPreviousTurnCapture(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-1", "Session"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	requestStore := databases.NewMemoryLLMRequestStore(chatStore)
	eng := &agent.Engine{
		LLM:       &llmCaptureBoundaryProvider{},
		Model:     "gpt-test",
		AgentRole: "fable",
	}

	attachLLMRequestCapture(eng, llmRequestCaptureConfig{
		Store:               requestStore,
		SessionID:           "sess-1",
		RunID:               "run-1",
		MessageID:           "assistant-1",
		ParentUserMessageID: "user-1",
		SpecialistID:        "fable",
	})
	if _, err := eng.LLM.Chat(ctx, []llm.Message{{Role: "user", Content: "turn one"}}, nil, "gpt-test"); err != nil {
		t.Fatalf("Chat turn one: %v", err)
	}

	attachLLMRequestCapture(eng, llmRequestCaptureConfig{
		Store:               requestStore,
		SessionID:           "sess-1",
		RunID:               "run-2",
		MessageID:           "assistant-2",
		ParentUserMessageID: "user-2",
		SpecialistID:        "fable",
	})
	if _, err := eng.LLM.Chat(ctx, []llm.Message{{Role: "user", Content: "turn two"}}, nil, "gpt-test"); err != nil {
		t.Fatalf("Chat turn two: %v", err)
	}

	firstTurn, err := requestStore.ListLLMRequestsForMessage(ctx, nil, "sess-1", "assistant-1")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage assistant-1: %v", err)
	}
	secondTurn, err := requestStore.ListLLMRequestsForMessage(ctx, nil, "sess-1", "assistant-2")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage assistant-2: %v", err)
	}
	if len(firstTurn) != 1 || len(secondTurn) != 1 {
		t.Fatalf("expected one request per turn, got first=%d second=%d", len(firstTurn), len(secondTurn))
	}

	var firstPayload, secondPayload struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(firstTurn[0].Payload, &firstPayload); err != nil {
		t.Fatalf("unmarshal first payload: %v", err)
	}
	if err := json.Unmarshal(secondTurn[0].Payload, &secondPayload); err != nil {
		t.Fatalf("unmarshal second payload: %v", err)
	}
	if len(firstPayload.Messages) != 1 || firstPayload.Messages[0].Content != "turn one" {
		t.Fatalf("expected first turn context to remain isolated, got %#v", firstPayload.Messages)
	}
	if len(secondPayload.Messages) != 1 || secondPayload.Messages[0].Content != "turn two" {
		t.Fatalf("expected second turn context, got %#v", secondPayload.Messages)
	}
}

// TestConfigureDurableChatEngineAttachesLLMRequestCapture guards the durable chat
// execution path (the path the chat UI actually uses) so provider-boundary context
// is captured and surfaced by the Context inspector. Regression for capture being
// wired only into the non-durable executeStreamChat/executeInternalJSONChat paths.
func TestConfigureDurableChatEngineAttachesLLMRequestCapture(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-1", "Session"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	requestStore := databases.NewMemoryLLMRequestStore(chatStore)

	eng := &agent.Engine{
		LLM:       &llmCaptureBoundaryProvider{},
		Model:     "gpt-test",
		AgentRole: "fable",
	}
	a := &app{llmRequestStore: requestStore}
	prepared := durableChatPreparedRun{
		exec: chatExecutionRequest{
			Engine: eng,
			RunRequest: chatRunRequest{
				SessionID:          "sess-1",
				UserMessageID:      "user-1",
				AssistantMessageID: "assistant-1",
			},
		},
	}

	a.configureDurableChatEngine(durable.Task{ID: "run-1"}, prepared, nil, nil)

	if _, err := eng.LLM.Chat(ctx, []llm.Message{
		{Role: "system", Content: "final system context after all manifold systems ran"},
		{Role: "user", Content: "final user prompt"},
	}, nil, "gpt-test"); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	requests, err := requestStore.ListLLMRequestsForMessage(ctx, nil, "sess-1", "assistant-1")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected durable chat path to capture one LLM request, got %d", len(requests))
	}
	if requests[0].MessageID != "assistant-1" || requests[0].ParentUserMessageID != "user-1" {
		t.Fatalf("unexpected associations message=%q parent=%q", requests[0].MessageID, requests[0].ParentUserMessageID)
	}
}
