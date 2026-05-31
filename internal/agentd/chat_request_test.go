package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

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
		ObjectiveID:  " objective-1 ",
		RoomID:       " room-1 ",
		RouteTarget:  " @gpt_bot:matrix.example.com ",
		SystemPrompt: "  custom system  ",
		ImageSize:    " 1024x1024 ",
	}

	req.normalize()

	if req.SessionID != normalizeClientChatSessionID("default") {
		t.Fatalf("expected normalized default session, got %q", req.SessionID)
	}
	if req.ProjectID != "project-1" {
		t.Fatalf("expected trimmed project id, got %q", req.ProjectID)
	}
	if req.ObjectiveID != "objective-1" {
		t.Fatalf("expected trimmed objective id, got %q", req.ObjectiveID)
	}
	if req.RoomID != "room-1" {
		t.Fatalf("expected trimmed room id, got %q", req.RoomID)
	}
	if req.RouteTarget != "@gpt_bot:matrix.example.com" {
		t.Fatalf("expected trimmed route target, got %q", req.RouteTarget)
	}
	if req.SystemPrompt != "custom system" {
		t.Fatalf("expected trimmed system prompt, got %q", req.SystemPrompt)
	}
	if req.ImageSize != "1024x1024" {
		t.Fatalf("expected trimmed image size, got %q", req.ImageSize)
	}
}

func TestNormalizeClientChatSessionID(t *testing.T) {
	t.Parallel()

	defaultID := normalizeClientChatSessionID("default")
	if defaultID == "default" || defaultID == "" {
		t.Fatalf("expected default alias to map to a UUID, got %q", defaultID)
	}
	if _, err := uuid.Parse(defaultID); err != nil {
		t.Fatalf("expected default alias to map to a UUID, got %q: %v", defaultID, err)
	}
	if got := normalizeClientChatSessionID("  default  "); got != defaultID {
		t.Fatalf("expected trimmed default alias to map deterministically, got %q want %q", got, defaultID)
	}
	if got := normalizeClientChatSessionID("alias-1"); got != normalizeClientChatSessionID("alias-1") {
		t.Fatalf("expected alias mapping to be deterministic")
	}
	const uuidID = "11111111-1111-4111-8111-111111111111"
	if got := normalizeClientChatSessionID(uuidID); got != uuidID {
		t.Fatalf("expected UUID to pass through unchanged, got %q", got)
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

	req := chatRunRequest{SessionID: "session-1", ProjectID: "project-1", RoomID: "room-1", RouteTarget: "@gpt_bot:matrix.example.com"}
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
	if got, ok := sandbox.RouteTargetFromContext(httpReq.Context()); !ok || got != "@gpt_bot:matrix.example.com" {
		t.Fatalf("expected route target on context, got %q ok=%v", got, ok)
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

func TestChatRunRequestUnmarshalAcceptsLegacyBotID(t *testing.T) {
	t.Parallel()

	var req chatRunRequest
	if err := json.Unmarshal([]byte(`{"prompt":"hello","bot_id":"@legacy:matrix.example.com"}`), &req); err != nil {
		t.Fatalf("unmarshal legacy bot_id: %v", err)
	}
	if req.RouteTarget != "@legacy:matrix.example.com" {
		t.Fatalf("expected route target from legacy bot_id, got %q", req.RouteTarget)
	}
}

func TestChatRunRequestUnmarshalMemorySettings(t *testing.T) {
	t.Parallel()

	var req chatRunRequest
	if err := json.Unmarshal([]byte(`{"prompt":"hello","evolving_memory_enabled":false,"beliefMemoryEnabled":true}`), &req); err != nil {
		t.Fatalf("unmarshal memory settings: %v", err)
	}
	if req.EvolvingMemoryEnabled == nil || *req.EvolvingMemoryEnabled {
		t.Fatalf("expected evolving memory false, got %#v", req.EvolvingMemoryEnabled)
	}
	if req.BeliefMemoryEnabled == nil || !*req.BeliefMemoryEnabled {
		t.Fatalf("expected belief memory true, got %#v", req.BeliefMemoryEnabled)
	}
	settings := chatMemorySettingsFromRunRequest(req)
	if settings.MemoryEnabled || settings.EvolvingMemoryEnabled || settings.BeliefMemoryEnabled {
		t.Fatalf("expected legacy mixed memory aliases to resolve off, got %+v", settings)
	}

	var unified chatRunRequest
	if err := json.Unmarshal([]byte(`{"prompt":"hello","memory_enabled":true}`), &unified); err != nil {
		t.Fatalf("unmarshal unified memory setting: %v", err)
	}
	settings = chatMemorySettingsFromRunRequest(unified)
	if !settings.MemoryEnabled || !settings.EvolvingMemoryEnabled || !settings.BeliefMemoryEnabled {
		t.Fatalf("expected unified memory setting to enable all memory systems, got %+v", settings)
	}
}
