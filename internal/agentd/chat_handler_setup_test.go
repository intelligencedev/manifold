package agentd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/agent/memory"
	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
)

func TestChatRequestOwnerPrefersCurrentUser(t *testing.T) {
	t.Parallel()

	userID := int64(7)
	owner := chatRequestOwner(&auth.User{ID: 42}, &userID)
	if owner != 42 {
		t.Fatalf("expected current user owner, got %d", owner)
	}
}

func TestPrepareChatHandlerStateUsesCurrentUserWhenAccessResolutionReturnsNil(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "ok"}}
	a := &app{
		cfg:              &config.Config{Auth: config.AuthConfig{Enabled: true}},
		llm:              baseProvider,
		baseToolRegistry: tools.NewRegistry(),
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		workspaceManager: stubWorkspaceManager{},
	}

	req := chatRunRequest{Prompt: "hello", SessionID: "sess-1"}
	httpReq := httptest.NewRequest(http.MethodPost, "/agent/run", nil).WithContext(auth.WithUser(httptest.NewRequest(http.MethodPost, "/agent/run", nil).Context(), &auth.User{ID: 42}))
	rr := httptest.NewRecorder()

	state, ok := a.prepareChatHandlerState(rr, httpReq, req)
	if !ok {
		t.Fatalf("expected prepareChatHandlerState to succeed: %d %s", rr.Code, rr.Body.String())
	}
	if state.UserID != nil {
		t.Fatalf("expected resolved chat access to remain nil when authStore is absent")
	}
	if state.Owner != 42 {
		t.Fatalf("expected owner to fall back to current user, got %d", state.Owner)
	}
}

func TestPrepareChatHandlerStateAppliesImagePrompt(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "ok"}}
	a := &app{
		cfg:              &config.Config{},
		llm:              baseProvider,
		baseToolRegistry: tools.NewRegistry(),
		chatStore:        chatStore,
		chatMemory:       memory.NewManager(chatStore, baseProvider, memory.Config{}),
		workspaceManager: stubWorkspaceManager{},
	}

	req := chatRunRequest{Prompt: "draw", SessionID: "sess-2", Image: true, ImageSize: "1024x1024"}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/prompt", nil)
	rr := httptest.NewRecorder()

	state, ok := a.prepareChatHandlerState(rr, httpReq, req)
	if !ok {
		t.Fatalf("expected prepareChatHandlerState to succeed: %d %s", rr.Code, rr.Body.String())
	}
	opts, ok := llm.ImagePromptFromContext(state.Request.Context())
	if !ok {
		t.Fatal("expected image prompt options on request context")
	}
	if opts.Size != "1024x1024" {
		t.Fatalf("expected image size 1024x1024, got %q", opts.Size)
	}
}
