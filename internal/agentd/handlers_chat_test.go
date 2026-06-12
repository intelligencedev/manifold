package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/projects"
)

func TestHydrateChatMessages_ToolMetadata(t *testing.T) {
	now := time.Now().UTC()
	raw := []persist.ChatMessage{
		{
			ID:        "a1",
			SessionID: "s",
			Role:      "assistant",
			Content:   `{"content":"Working","tool_calls":[{"name":"search_docs","id":"call-1","args":{"q":"foo"}}]}`,
			CreatedAt: now,
		},
		{
			ID:        "t1",
			SessionID: "s",
			Role:      "tool",
			Content:   `{"content":"result text","tool_id":"call-1"}`,
			CreatedAt: now,
		},
	}

	hydrated := hydrateChatMessages(raw)
	if len(hydrated) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(hydrated))
	}

	if hydrated[0].Content != "Working" {
		t.Fatalf("assistant content not stripped: %q", hydrated[0].Content)
	}

	tool := hydrated[1]
	if tool.Title != "search_docs" {
		t.Fatalf("expected tool title 'search_docs', got %q", tool.Title)
	}
	if tool.ToolArgs != `{"q":"foo"}` {
		t.Fatalf("expected tool args JSON, got %q", tool.ToolArgs)
	}
	if tool.ToolID != "call-1" {
		t.Fatalf("expected tool ID propagated, got %q", tool.ToolID)
	}
	if tool.Content != "result text" {
		t.Fatalf("expected tool content, got %q", tool.Content)
	}
}

func TestHydrateChatMessages_SkipsToolCallOnlyAssistant(t *testing.T) {
	now := time.Now().UTC()
	raw := []persist.ChatMessage{
		{
			ID:        "a1",
			SessionID: "s",
			Role:      "assistant",
			Content:   `{"content":"","tool_calls":[{"name":"search","id":"call-1","args":{"q":"x"}}]}`,
			CreatedAt: now,
		},
		{
			ID:        "t1",
			SessionID: "s",
			Role:      "tool",
			Content:   `{"content":"ok","tool_id":"call-1"}`,
			CreatedAt: now,
		},
	}

	hydrated := hydrateChatMessages(raw)
	if len(hydrated) != 1 {
		t.Fatalf("expected only tool message to remain, got %d", len(hydrated))
	}
	if hydrated[0].Role != "tool" {
		t.Fatalf("expected remaining message to be tool, got %s", hydrated[0].Role)
	}
}

func TestHydrateChatMessages_IgnoresPlainMessages(t *testing.T) {
	now := time.Now().UTC()
	raw := []persist.ChatMessage{{
		ID:        "u1",
		SessionID: "s",
		Role:      "user",
		Content:   "hello",
		CreatedAt: now,
	}}

	hydrated := hydrateChatMessages(raw)
	if len(hydrated) != 1 {
		t.Fatalf("expected 1 message, got %d", len(hydrated))
	}
	if hydrated[0].Content != "hello" {
		t.Fatalf("unexpected content: %q", hydrated[0].Content)
	}
	if hydrated[0].Title != "" || hydrated[0].ToolArgs != "" || hydrated[0].ToolID != "" {
		t.Fatalf("expected no tool metadata on plain message")
	}
}

func TestRelatedToolMessageIDs(t *testing.T) {
	now := time.Now().UTC()
	msgs := []persist.ChatMessage{
		{
			ID:        "assistant-1",
			SessionID: "s",
			Role:      "assistant",
			Content:   `{"content":"Working","tool_calls":[{"name":"search_docs","id":"call-1","args":{"q":"foo"}},{"name":"lookup","id":"call-2","args":{"q":"bar"}}]}`,
			CreatedAt: now,
		},
		{
			ID:        "tool-1",
			SessionID: "s",
			Role:      "tool",
			Content:   `{"content":"result 1","tool_id":"call-1"}`,
			CreatedAt: now.Add(time.Second),
		},
		{
			ID:        "tool-2",
			SessionID: "s",
			Role:      "tool",
			Content:   `{"content":"result 2","tool_id":"call-2"}`,
			CreatedAt: now.Add(2 * time.Second),
		},
		{
			ID:        "tool-3",
			SessionID: "s",
			Role:      "tool",
			Content:   `{"content":"ignored","tool_id":"call-3"}`,
			CreatedAt: now.Add(3 * time.Second),
		},
	}

	related := relatedToolMessageIDs(msgs, msgs[0])
	if len(related) != 2 {
		t.Fatalf("expected 2 related tool messages, got %d", len(related))
	}
	if related[0] != "tool-1" || related[1] != "tool-2" {
		t.Fatalf("unexpected related tool messages: %#v", related)
	}
}

func TestDeleteChatSessionDeletesTemporaryProject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	projectService := projects.NewService(t.TempDir(), "")
	project, err := projectService.CreateProjectKind(ctx, systemUserID, "Temporary Chat", projects.ProjectKindTemporary)
	if err != nil {
		t.Fatalf("CreateProjectKind: %v", err)
	}
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-temp", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := chatStore.SetSessionProject(ctx, nil, "sess-temp", project.ID); err != nil {
		t.Fatalf("SetSessionProject: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore, projectsService: projectService}
	req := httptest.NewRequest(http.MethodDelete, "/api/chat/sessions/sess-temp", nil)
	rr := httptest.NewRecorder()

	a.chatSessionDetailHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	temporaryProjects, err := projectService.ListProjectsByKind(ctx, systemUserID, projects.ProjectKindTemporary)
	if err != nil {
		t.Fatalf("ListProjectsByKind: %v", err)
	}
	if len(temporaryProjects) != 0 {
		t.Fatalf("expected temporary project to be deleted, got %#v", temporaryProjects)
	}
}

func TestDeleteChatSessionPreservesUserProject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	projectService := projects.NewService(t.TempDir(), "")
	project, err := projectService.CreateProject(ctx, systemUserID, "Persistent Project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-user-project", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := chatStore.SetSessionProject(ctx, nil, "sess-user-project", project.ID); err != nil {
		t.Fatalf("SetSessionProject: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore, projectsService: projectService}
	req := httptest.NewRequest(http.MethodDelete, "/api/chat/sessions/sess-user-project", nil)
	rr := httptest.NewRecorder()

	a.chatSessionDetailHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	userProjects, err := projectService.ListProjects(ctx, systemUserID)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(userProjects) != 1 || userProjects[0].ID != project.ID {
		t.Fatalf("expected user project to remain, got %#v", userProjects)
	}
}

func TestDeleteChatSessionDeletesCommandPolicyOverride(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-policy-delete", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	commandPolicyStore := databases.NewCommandPolicyStore(nil)
	if err := commandPolicyStore.Init(ctx); err != nil {
		t.Fatalf("init command policy store: %v", err)
	}
	if err := commandPolicyStore.SetSessionAllowAll(ctx, systemUserID, "sess-policy-delete", true); err != nil {
		t.Fatalf("SetSessionAllowAll: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore, commandPolicyStore: commandPolicyStore}
	req := httptest.NewRequest(http.MethodDelete, "/api/chat/sessions/sess-policy-delete", nil)
	rr := httptest.NewRecorder()

	a.chatSessionDetailHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if _, ok, err := commandPolicyStore.GetSessionOverride(ctx, systemUserID, "sess-policy-delete"); err != nil {
		t.Fatalf("GetSessionOverride: %v", err)
	} else if ok {
		t.Fatal("expected command policy session override to be deleted")
	}
}

func TestPatchChatSessionCommandPolicyAllowAll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-policy-patch", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	commandPolicyStore := databases.NewCommandPolicyStore(nil)
	if err := commandPolicyStore.Init(ctx); err != nil {
		t.Fatalf("init command policy store: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore, commandPolicyStore: commandPolicyStore}

	enableReq := httptest.NewRequest(http.MethodPatch, "/api/chat/sessions/sess-policy-patch", strings.NewReader(`{"commandPolicyAllowAll":true}`))
	enableRR := httptest.NewRecorder()
	a.chatSessionDetailHandler().ServeHTTP(enableRR, enableReq)
	if enableRR.Code != http.StatusOK {
		t.Fatalf("expected enable status 200, got %d: %s", enableRR.Code, enableRR.Body.String())
	}
	enabled := decodeChatSessionResponse(t, enableRR)
	if !enabled.CommandPolicyAllowAll {
		t.Fatalf("expected commandPolicyAllowAll=true in response, got %+v", enabled)
	}
	if override, ok, err := commandPolicyStore.GetSessionOverride(ctx, systemUserID, "sess-policy-patch"); err != nil {
		t.Fatalf("GetSessionOverride enabled: %v", err)
	} else if !ok || !override.AllowAllCommands {
		t.Fatalf("expected stored allow-all override, ok=%v override=%+v", ok, override)
	}

	disableReq := httptest.NewRequest(http.MethodPatch, "/api/chat/sessions/sess-policy-patch", strings.NewReader(`{"commandPolicyAllowAll":false}`))
	disableRR := httptest.NewRecorder()
	a.chatSessionDetailHandler().ServeHTTP(disableRR, disableReq)
	if disableRR.Code != http.StatusOK {
		t.Fatalf("expected disable status 200, got %d: %s", disableRR.Code, disableRR.Body.String())
	}
	disabled := decodeChatSessionResponse(t, disableRR)
	if disabled.CommandPolicyAllowAll {
		t.Fatalf("expected commandPolicyAllowAll=false in response, got %+v", disabled)
	}
	if _, ok, err := commandPolicyStore.GetSessionOverride(ctx, systemUserID, "sess-policy-patch"); err != nil {
		t.Fatalf("GetSessionOverride disabled: %v", err)
	} else if ok {
		t.Fatal("expected command policy session override to be cleared")
	}
}

func TestPatchChatSessionPinned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-pin-patch", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore}

	pinReq := httptest.NewRequest(http.MethodPatch, "/api/chat/sessions/sess-pin-patch", strings.NewReader(`{"pinned":true}`))
	pinRR := httptest.NewRecorder()
	a.chatSessionDetailHandler().ServeHTTP(pinRR, pinReq)
	if pinRR.Code != http.StatusOK {
		t.Fatalf("expected pin status 200, got %d: %s", pinRR.Code, pinRR.Body.String())
	}
	pinned := decodeChatSessionResponse(t, pinRR)
	if !pinned.Pinned {
		t.Fatalf("expected pinned=true in response, got %+v", pinned)
	}

	unpinReq := httptest.NewRequest(http.MethodPatch, "/api/chat/sessions/sess-pin-patch", strings.NewReader(`{"pinned":false}`))
	unpinRR := httptest.NewRecorder()
	a.chatSessionDetailHandler().ServeHTTP(unpinRR, unpinReq)
	if unpinRR.Code != http.StatusOK {
		t.Fatalf("expected unpin status 200, got %d: %s", unpinRR.Code, unpinRR.Body.String())
	}
	unpinned := decodeChatSessionResponse(t, unpinRR)
	if unpinned.Pinned {
		t.Fatalf("expected pinned=false in response, got %+v", unpinned)
	}
}

func TestPatchChatSessionActiveTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-target-patch", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore}

	req := httptest.NewRequest(http.MethodPatch, "/api/chat/sessions/sess-target-patch", strings.NewReader(`{"activeSpecialist":" planner ","activeTeam":" ops "}`))
	rr := httptest.NewRecorder()
	a.chatSessionDetailHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	updated := decodeChatSessionResponse(t, rr)
	if updated.ActiveSpecialist != "planner" || updated.ActiveTeam != "ops" {
		t.Fatalf("expected trimmed active target in response, got specialist=%q team=%q", updated.ActiveSpecialist, updated.ActiveTeam)
	}

	session, err := chatStore.GetSession(ctx, nil, "sess-target-patch")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.ActiveSpecialist != "planner" || session.ActiveTeam != "ops" {
		t.Fatalf("expected active target to persist, got specialist=%q team=%q", session.ActiveSpecialist, session.ActiveTeam)
	}
}

func decodeChatSessionResponse(t *testing.T, rr *httptest.ResponseRecorder) persist.ChatSession {
	t.Helper()
	var session persist.ChatSession
	if err := json.Unmarshal(rr.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode chat session response: %v\n%s", err, rr.Body.String())
	}
	return session
}

func TestPatchChatSessionProjectDeletesPreviousTemporaryProject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	projectService := projects.NewService(t.TempDir(), "")
	temporaryProject, err := projectService.CreateProjectKind(ctx, systemUserID, "Temporary Chat", projects.ProjectKindTemporary)
	if err != nil {
		t.Fatalf("CreateProjectKind: %v", err)
	}
	persistentProject, err := projectService.CreateProject(ctx, systemUserID, "Persistent Project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-switch-temp", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := chatStore.SetSessionProject(ctx, nil, "sess-switch-temp", temporaryProject.ID); err != nil {
		t.Fatalf("SetSessionProject: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore, projectsService: projectService}
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/chat/sessions/sess-switch-temp",
		strings.NewReader(`{"projectId":"`+persistentProject.ID+`"}`),
	)
	rr := httptest.NewRecorder()

	a.chatSessionDetailHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	session, err := chatStore.GetSession(ctx, nil, "sess-switch-temp")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.ProjectID != persistentProject.ID {
		t.Fatalf("expected session project %q, got %q", persistentProject.ID, session.ProjectID)
	}
	temporaryProjects, err := projectService.ListProjectsByKind(ctx, systemUserID, projects.ProjectKindTemporary)
	if err != nil {
		t.Fatalf("ListProjectsByKind: %v", err)
	}
	if len(temporaryProjects) != 0 {
		t.Fatalf("expected temporary project to be deleted, got %#v", temporaryProjects)
	}
	userProjects, err := projectService.ListProjects(ctx, systemUserID)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(userProjects) != 1 || userProjects[0].ID != persistentProject.ID {
		t.Fatalf("expected persistent project to remain, got %#v", userProjects)
	}
}

func TestPatchChatSessionProjectPreservesPreviousUserProject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	chatStore := newPromptHandlerChatStore()
	projectService := projects.NewService(t.TempDir(), "")
	previousProject, err := projectService.CreateProject(ctx, systemUserID, "Previous Project")
	if err != nil {
		t.Fatalf("CreateProject previous: %v", err)
	}
	nextProject, err := projectService.CreateProject(ctx, systemUserID, "Next Project")
	if err != nil {
		t.Fatalf("CreateProject next: %v", err)
	}
	if _, err := chatStore.EnsureSession(ctx, nil, "sess-switch-user", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if _, err := chatStore.SetSessionProject(ctx, nil, "sess-switch-user", previousProject.ID); err != nil {
		t.Fatalf("SetSessionProject: %v", err)
	}
	a := &app{cfg: &config.Config{}, chatStore: chatStore, projectsService: projectService}
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/chat/sessions/sess-switch-user",
		strings.NewReader(`{"projectId":"`+nextProject.ID+`"}`),
	)
	rr := httptest.NewRecorder()

	a.chatSessionDetailHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	userProjects, err := projectService.ListProjects(ctx, systemUserID)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	seen := make(map[string]bool, len(userProjects))
	for _, project := range userProjects {
		seen[project.ID] = true
	}
	if !seen[previousProject.ID] || !seen[nextProject.ID] {
		t.Fatalf("expected both user projects to remain, got %#v", userProjects)
	}
}
