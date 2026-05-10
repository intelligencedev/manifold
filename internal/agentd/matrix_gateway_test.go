package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
	"manifold/internal/matrixgw"
	"manifold/internal/projects"
	"manifold/internal/workspaces"
)

type fakeGatewayClient struct {
	sentText []string
	sentHTML []string
}

func (f *fakeGatewayClient) Sync(context.Context, string, int, string) (matrixgw.SyncResponse, error) {
	return matrixgw.SyncResponse{}, context.Canceled
}

func (f *fakeGatewayClient) JoinRoom(context.Context, string) error { return nil }

func (f *fakeGatewayClient) SendText(_ context.Context, _ string, text string) error {
	f.sentText = append(f.sentText, text)
	return nil
}

func (f *fakeGatewayClient) SendFormattedText(_ context.Context, _ string, text, _ string) error {
	f.sentHTML = append(f.sentHTML, text)
	return nil
}

func TestHandleMatrixMessageRunsSpecialistTarget(t *testing.T) {
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
	service, err := matrixgw.New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms:         []config.MatrixRoomConfig{{RoomID: "!room:test"}},
	})
	if err != nil {
		t.Fatalf("matrixgw.New() error = %v", err)
	}
	client := &fakeGatewayClient{}
	service.SetSyncClient(client)
	a.matrixGateway = service
	store := a.chatStore.(*promptHandlerChatStore)

	err = a.handleMatrixMessage(context.Background(), matrixgw.InboundMessage{
		RoomID:  "!room:test",
		EventID: "$event-1",
		Sender:  "@user:test",
		Body:    "@weather forecast please",
		Prompt:  "forecast please",
		Target:  "weather",
	})
	if err != nil {
		t.Fatalf("handleMatrixMessage() error = %v", err)
	}

	sessionID := matrixSessionID("!room:test", "weather")
	messages := store.messages[sessionID]
	if len(messages) < 2 {
		t.Fatalf("expected stored chat turn, got %#v", messages)
	}
	if got := messages[len(messages)-1].Content; got != "specialist response" {
		t.Fatalf("expected stored assistant response, got %q", got)
	}
	if len(client.sentHTML) != 1 || client.sentHTML[0] != "weather: specialist response" {
		t.Fatalf("expected attributed Matrix reply to be sent, got %#v", client.sentHTML)
	}
}

func TestEnsureMatrixRoomProjectReusesSystemProject(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	a := &app{
		cfg:             &config.Config{Workdir: workdir},
		projectsService: projects.NewService(workdir, ""),
	}

	projectID, err := a.ensureMatrixRoomProject(context.Background(), "!room:test")
	if err != nil {
		t.Fatalf("ensureMatrixRoomProject() error = %v", err)
	}
	if projectID == "" {
		t.Fatal("expected project id to be created")
	}

	reusedProjectID, err := a.ensureMatrixRoomProject(context.Background(), "!room:test")
	if err != nil {
		t.Fatalf("ensureMatrixRoomProject() reuse error = %v", err)
	}
	if reusedProjectID != projectID {
		t.Fatalf("expected project id %q to be reused, got %q", projectID, reusedProjectID)
	}

	list, err := a.projectsService.ListProjects(context.Background(), systemUserID)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one system project, got %d", len(list))
	}
	if list[0].ID != projectID {
		t.Fatalf("expected listed project id %q, got %q", projectID, list[0].ID)
	}
	if list[0].Name != matrixRoomProjectName("!room:test") {
		t.Fatalf("expected project name %q, got %q", matrixRoomProjectName("!room:test"), list[0].Name)
	}
}

func TestHandleMatrixMessageAutoCreatesRoomProject(t *testing.T) {
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
	workdir := t.TempDir()
	a.cfg.Workdir = workdir
	a.projectsService = projects.NewService(workdir, "")
	realWorkspaceManager := workspaces.NewManager(a.cfg)
	var gotProjectID string
	a.workspaceManager = stubWorkspaceManager{checkout: func(ctx context.Context, userID int64, projectID, sessionID string) (workspaces.Workspace, error) {
		gotProjectID = projectID
		return realWorkspaceManager.Checkout(ctx, userID, projectID, sessionID)
	}}

	service, err := matrixgw.New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms:         []config.MatrixRoomConfig{{RoomID: "!room:test"}},
	})
	if err != nil {
		t.Fatalf("matrixgw.New() error = %v", err)
	}
	client := &fakeGatewayClient{}
	service.SetSyncClient(client)
	a.matrixGateway = service

	err = a.handleMatrixMessage(context.Background(), matrixgw.InboundMessage{
		RoomID:  "!room:test",
		EventID: "$event-1",
		Sender:  "@user:test",
		Body:    "@weather forecast please",
		Prompt:  "forecast please",
		Target:  "weather",
	})
	if err != nil {
		t.Fatalf("handleMatrixMessage() error = %v", err)
	}
	if gotProjectID == "" {
		t.Fatal("expected matrix run to resolve a project id before checkout")
	}

	list, err := a.projectsService.ListProjects(context.Background(), systemUserID)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one system project, got %d", len(list))
	}
	if list[0].ID != gotProjectID {
		t.Fatalf("expected created project id %q, got %q", gotProjectID, list[0].ID)
	}
	if list[0].Name != matrixRoomProjectName("!room:test") {
		t.Fatalf("expected project name %q, got %q", matrixRoomProjectName("!room:test"), list[0].Name)
	}
}
