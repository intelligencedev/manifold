package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/config"
	persist "manifold/internal/persistence"
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
