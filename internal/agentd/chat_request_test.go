package agentd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"manifold/internal/config"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

type stubWorkspaceManager struct {
	checkout func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error)
}

func (s stubWorkspaceManager) Checkout(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
	if s.checkout != nil {
		return s.checkout(ctx, userID, projectID, sessionID)
	}
	return workspaces.Workspace{}, nil
}

func (stubWorkspaceManager) Commit(context.Context, workspaces.Workspace) error  { return nil }
func (stubWorkspaceManager) Cleanup(context.Context, workspaces.Workspace) error { return nil }
func (stubWorkspaceManager) Mode() string                                        { return "stub" }

func TestChatRunRequestNormalize(t *testing.T) {
	t.Parallel()

	req := chatRunRequest{
		SessionID:    "  ",
		ProjectID:    "  project-1  ",
		RoomID:       " room-1 ",
		SystemPrompt: "  custom system  ",
		ImageSize:    " 1024x1024 ",
	}

	req.normalize()

	if req.SessionID != "default" {
		t.Fatalf("expected default session, got %q", req.SessionID)
	}
	if req.ProjectID != "project-1" {
		t.Fatalf("expected trimmed project id, got %q", req.ProjectID)
	}
	if req.RoomID != "room-1" {
		t.Fatalf("expected trimmed room id, got %q", req.RoomID)
	}
	if req.SystemPrompt != "custom system" {
		t.Fatalf("expected trimmed system prompt, got %q", req.SystemPrompt)
	}
	if req.ImageSize != "1024x1024" {
		t.Fatalf("expected trimmed image size, got %q", req.ImageSize)
	}
}

func TestResolveChatDispatchTargetPrefersCanonicalTeam(t *testing.T) {
	t.Parallel()

	query := make(url.Values)
	query.Set("specialist", "writer")
	query.Set("team", "alpha")
	query.Set("group", "legacy")

	target := resolveChatDispatchTarget(query)

	if target.SpecialistName != "writer" {
		t.Fatalf("expected specialist writer, got %q", target.SpecialistName)
	}
	if target.TeamName != "alpha" {
		t.Fatalf("expected canonical team to win, got %q", target.TeamName)
	}
}

func TestPrepareChatRunRequestAttachesContextAndWorkspace(t *testing.T) {
	t.Parallel()

	var gotUserID int64
	var gotProjectID string
	var gotSessionID string
	a := &app{
		cfg: &config.Config{Auth: config.AuthConfig{Enabled: true, CookieName: "auth_cookie"}},
		workspaceManager: stubWorkspaceManager{checkout: func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
			gotUserID = userID
			gotProjectID = projectID
			gotSessionID = sessionID
			return workspaces.Workspace{UserID: userID, ProjectID: projectID, SessionID: sessionID, BaseDir: "/tmp/project-1"}, nil
		}},
	}

	req := chatRunRequest{SessionID: "session-1", ProjectID: "project-1", RoomID: "room-1"}
	httpReq := httptest.NewRequest(http.MethodPost, "/agent/run", nil)
	httpReq.AddCookie(&http.Cookie{Name: "auth_cookie", Value: "secret"})
	userID := int64(42)

	httpReq, ws, statusCode, err := a.prepareChatRunRequest(httpReq, &userID, req)
	if err != nil {
		t.Fatalf("prepareChatRunRequest returned error: %v", err)
	}
	if statusCode != 0 {
		t.Fatalf("expected zero status, got %d", statusCode)
	}
	if ws == nil || ws.BaseDir != "/tmp/project-1" {
		t.Fatalf("expected checked out workspace, got %#v", ws)
	}
	if gotUserID != 42 || gotProjectID != "project-1" || gotSessionID != "session-1" {
		t.Fatalf("unexpected checkout args: user=%d project=%q session=%q", gotUserID, gotProjectID, gotSessionID)
	}
	if got, ok := sandbox.SessionIDFromContext(httpReq.Context()); !ok || got != "session-1" {
		t.Fatalf("expected session id on context, got %q ok=%v", got, ok)
	}
	if got, ok := sandbox.ProjectIDFromContext(httpReq.Context()); !ok || got != "project-1" {
		t.Fatalf("expected project id on context, got %q ok=%v", got, ok)
	}
	if got, ok := sandbox.RoomIDFromContext(httpReq.Context()); !ok || got != "room-1" {
		t.Fatalf("expected room id on context, got %q ok=%v", got, ok)
	}
	if got, ok := sandbox.BaseDirFromContext(httpReq.Context()); !ok || got != "/tmp/project-1" {
		t.Fatalf("expected base dir on context, got %q ok=%v", got, ok)
	}
	if got, ok := sandbox.AuthCookieFromContext(httpReq.Context()); !ok || got != "auth_cookie=secret" {
		t.Fatalf("expected auth cookie on context, got %q ok=%v", got, ok)
	}
	if outbox, ok := sandbox.MatrixOutboxFromContext(httpReq.Context()); !ok || outbox == nil {
		t.Fatalf("expected matrix outbox on context")
	}
}

func TestPrepareChatRunRequestMapsWorkspaceErrors(t *testing.T) {
	t.Parallel()

	a := &app{
		cfg: &config.Config{},
		workspaceManager: stubWorkspaceManager{checkout: func(context.Context, int64, string, string) (workspaces.Workspace, error) {
			return workspaces.Workspace{}, workspaces.ErrInvalidProjectID
		}},
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/api/prompt", nil)
	_, _, statusCode, err := a.prepareChatRunRequest(httpReq, nil, chatRunRequest{SessionID: "default", ProjectID: "../bad"})
	if !errors.Is(err, workspaces.ErrInvalidProjectID) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
	if statusCode != http.StatusBadRequest {
		t.Fatalf("expected bad request status, got %d", statusCode)
	}
}
