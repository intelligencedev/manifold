package agentd

import (
	"context"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/fleet"
	"manifold/internal/llm"
	"manifold/internal/persistence/databases"
)

func TestAttachChatEngineRuntimeInstallsCaptureAndFleetCallbacks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-1", "Session"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	requestStore := databases.NewMemoryLLMRequestStore(chatStore)
	bus := fleet.NewBus(8)
	eng := &agent.Engine{
		LLM:       &llmCaptureBoundaryProvider{},
		Model:     "gpt-test",
		AgentRole: "orchestrator",
	}

	attachChatEngineRuntime(ChatDeps{
		LLMRequestStore: requestStore,
		FleetBus:        bus,
	}, chatEngineAttachConfig{
		Engine: eng,
		Fleet: fleetCallbackRequest{
			RunID:     "run-1",
			SessionID: "sess-1",
			UserID:    int64Ptr(7),
		},
		Capture: llmRequestCaptureConfig{
			SessionID:           "sess-1",
			UserID:              int64Ptr(7),
			RunID:               "run-1",
			MessageID:           "assistant-1",
			ParentUserMessageID: "user-1",
		},
	})

	eng.OnToolStart("read_file", []byte(`{"path":"README.md"}`), "tool-1")
	if _, err := eng.LLM.Chat(ctx, []llm.Message{{Role: "user", Content: "hello"}}, nil, "gpt-test"); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	requests, err := requestStore.ListLLMRequestsForMessage(ctx, int64Ptr(7), "sess-1", "assistant-1")
	if err != nil {
		t.Fatalf("ListLLMRequestsForMessage: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("captured requests = %d, want 1", len(requests))
	}
	events := bus.Recent(7)
	if len(events) != 1 || events[0].Kind != fleet.EventToolStart {
		t.Fatalf("fleet events = %#v, want one tool-start event", events)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
