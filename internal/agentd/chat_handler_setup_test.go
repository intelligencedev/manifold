package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/agent/memory"
	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/projects"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
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

func TestPrepareChatHandlerStateUsesLockedSessionProject(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(context.Background(), nil, "sess-locked", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := chatStore.SetSessionProject(context.Background(), nil, "sess-locked", "project-locked"); err != nil {
		t.Fatalf("SetSessionProject: %v", err)
	}
	var gotProjectID string
	a := &app{
		cfg:       &config.Config{},
		chatStore: chatStore,
		workspaceManager: stubWorkspaceManager{checkout: func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
			gotProjectID = projectID
			return workspaces.Workspace{UserID: userID, ProjectID: projectID, SessionID: sessionID, BaseDir: "/tmp/project-locked"}, nil
		}},
	}

	req := chatRunRequest{Prompt: "continue", SessionID: "sess-locked"}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/prompt", nil)
	rr := httptest.NewRecorder()

	state, ok := a.prepareChatHandlerState(rr, httpReq, req)
	if !ok {
		t.Fatalf("expected prepareChatHandlerState to succeed: %d %s", rr.Code, rr.Body.String())
	}
	if gotProjectID != "project-locked" {
		t.Fatalf("expected checkout to use locked project, got %q", gotProjectID)
	}
	if state.RunRequest.ProjectID != "project-locked" {
		t.Fatalf("expected run request project to be locked project, got %q", state.RunRequest.ProjectID)
	}
}

func TestPrepareChatHandlerStateRejectsProjectMismatch(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(context.Background(), nil, "sess-locked", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := chatStore.SetSessionProject(context.Background(), nil, "sess-locked", "project-locked"); err != nil {
		t.Fatalf("SetSessionProject: %v", err)
	}
	called := false
	a := &app{
		cfg:       &config.Config{},
		chatStore: chatStore,
		workspaceManager: stubWorkspaceManager{checkout: func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
			called = true
			return workspaces.Workspace{}, nil
		}},
	}

	req := chatRunRequest{Prompt: "continue", SessionID: "sess-locked", ProjectID: "project-other"}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/prompt", nil)
	rr := httptest.NewRecorder()

	if _, ok := a.prepareChatHandlerState(rr, httpReq, req); ok {
		t.Fatal("expected prepareChatHandlerState to reject mismatched project")
	}
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	if called {
		t.Fatal("workspace checkout should not run on project mismatch")
	}
}

func TestPrepareChatHandlerStateLocksFirstRunProject(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	a := &app{
		cfg:       &config.Config{},
		chatStore: chatStore,
		workspaceManager: stubWorkspaceManager{checkout: func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
			return workspaces.Workspace{UserID: userID, ProjectID: projectID, SessionID: sessionID, BaseDir: "/tmp/project-1"}, nil
		}},
	}

	req := chatRunRequest{Prompt: "start", SessionID: "sess-new", ProjectID: "project-1"}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/prompt", nil)
	rr := httptest.NewRecorder()

	state, ok := a.prepareChatHandlerState(rr, httpReq, req)
	if !ok {
		t.Fatalf("expected prepareChatHandlerState to succeed: %d %s", rr.Code, rr.Body.String())
	}
	if state.RunRequest.ProjectID != "project-1" {
		t.Fatalf("expected run request project, got %q", state.RunRequest.ProjectID)
	}
	session, err := chatStore.GetSession(context.Background(), nil, "sess-new")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.ProjectID != "project-1" {
		t.Fatalf("expected session project lock, got %q", session.ProjectID)
	}
}

func TestPrepareChatHandlerStateCreatesTemporaryProjectForNewChat(t *testing.T) {
	t.Parallel()

	chatStore := newPromptHandlerChatStore()
	projectService := projects.NewService(t.TempDir(), "")
	var gotProjectID string
	a := &app{
		cfg:             &config.Config{},
		chatStore:       chatStore,
		projectsService: projectService,
		workspaceManager: stubWorkspaceManager{checkout: func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
			gotProjectID = projectID
			return workspaces.Workspace{UserID: userID, ProjectID: projectID, SessionID: sessionID, BaseDir: "/tmp/" + projectID}, nil
		}},
	}

	req := chatRunRequest{Prompt: "start", SessionID: "sess-temp"}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/prompt", nil)
	rr := httptest.NewRecorder()

	state, ok := a.prepareChatHandlerState(rr, httpReq, req)
	if !ok {
		t.Fatalf("expected prepareChatHandlerState to succeed: %d %s", rr.Code, rr.Body.String())
	}
	if state.RunRequest.ProjectID == "" {
		t.Fatal("expected run request to use a temporary project")
	}
	if gotProjectID != state.RunRequest.ProjectID {
		t.Fatalf("expected checkout project %q, got %q", state.RunRequest.ProjectID, gotProjectID)
	}
	session, err := chatStore.GetSession(context.Background(), nil, "sess-temp")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.ProjectID != state.RunRequest.ProjectID {
		t.Fatalf("expected session project lock %q, got %q", state.RunRequest.ProjectID, session.ProjectID)
	}
	temporaryProjects, err := projectService.ListProjectsByKind(context.Background(), systemUserID, projects.ProjectKindTemporary)
	if err != nil {
		t.Fatalf("ListProjectsByKind: %v", err)
	}
	if len(temporaryProjects) != 1 || temporaryProjects[0].ID != state.RunRequest.ProjectID {
		t.Fatalf("expected one temporary project matching session, got %#v", temporaryProjects)
	}
}
