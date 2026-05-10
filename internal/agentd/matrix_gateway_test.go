package agentd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/matrixgw"
	"manifold/internal/persistence"
	"manifold/internal/projects"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
)

type fakeGatewayClient struct {
	sentText []string
	sentHTML []string
	uploaded [][]byte
	images   []matrixgw.ImageMessage
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

func (f *fakeGatewayClient) UploadMedia(_ context.Context, content io.Reader, _ string, _ int64) (string, error) {
	body, err := io.ReadAll(content)
	if err != nil {
		return "", err
	}
	f.uploaded = append(f.uploaded, body)
	return "mxc://matrix.example.com/uploaded", nil
}

func (f *fakeGatewayClient) SendImage(_ context.Context, _ string, image matrixgw.ImageMessage) error {
	f.images = append(f.images, image)
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

func TestHandleMatrixMessageUploadsGeneratedImages(t *testing.T) {
	t.Parallel()

	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"unused","tool_calls":[]}}]}`))
	}))
	defer specialistServer.Close()

	a := newSpecialistTestApp(t, specialistServer.URL, nil)
	provider := &testhelpers.FakeProvider{Resp: llm.Message{
		Role:    "assistant",
		Content: "Generated image",
		Images:  []llm.GeneratedImage{{Data: []byte("pngbytes"), MIMEType: "image/png"}},
	}}
	baseTools := tools.NewRegistry()
	a.llm = provider
	a.baseToolRegistry = baseTools
	a.engine = &agent.Engine{LLM: provider, Tools: baseTools, Model: "orchestrator-model"}

	service, err := matrixgw.New(config.MatrixConfig{
		Enabled:       true,
		HomeserverURL: "https://matrix.example.com",
		UserID:        "@manifold:example.com",
		AccessToken:   "token",
		Rooms:         []config.MatrixRoomConfig{{RoomID: "!room:test", AllowUnmentioned: true}},
	})
	if err != nil {
		t.Fatalf("matrixgw.New() error = %v", err)
	}
	client := &fakeGatewayClient{}
	service.SetSyncClient(client)
	a.matrixGateway = service

	err = a.handleMatrixMessage(context.Background(), matrixgw.InboundMessage{
		RoomID:  "!room:test",
		EventID: "$event-2",
		Sender:  "@user:test",
		Body:    "draw a square",
		Prompt:  "draw a square",
	})
	if err != nil {
		t.Fatalf("handleMatrixMessage() error = %v", err)
	}
	if len(client.uploaded) != 1 || string(client.uploaded[0]) != "pngbytes" {
		t.Fatalf("expected one uploaded image payload, got %#v", client.uploaded)
	}
	if len(client.images) != 1 {
		t.Fatalf("expected one matrix image event, got %#v", client.images)
	}
	if client.images[0].Body == "" || client.images[0].URL != "mxc://matrix.example.com/uploaded" {
		t.Fatalf("unexpected matrix image event: %#v", client.images[0])
	}
	if client.images[0].MIMEType != "image/png" || client.images[0].Size != int64(len("pngbytes")) {
		t.Fatalf("unexpected matrix image metadata: %#v", client.images[0])
	}
	if len(client.sentHTML) != 1 {
		t.Fatalf("expected attributed Matrix reply to be sent, got %#v", client.sentHTML)
	}
	if !strings.Contains(client.sentHTML[0], "Generated image") || !strings.Contains(client.sentHTML[0], "Generated images:") {
		t.Fatalf("expected image summary in Matrix reply, got %q", client.sentHTML[0])
	}
}

func TestHandleMatrixMessageUsesImageAPIForImageGenerationSpecialist(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody map[string]any
	specialistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/images/generations" {
			http.Error(w, "unexpected chat request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"cG5nYnl0ZXM="}]}`))
	}))
	defer specialistServer.Close()

	a := newSpecialistTestApp(t, specialistServer.URL, []config.SpecialistConfig{{
		Name:            "image-maker",
		Description:     "Image generation specialist",
		Provider:        "openai",
		BaseURL:         specialistServer.URL,
		APIKey:          "test",
		Model:           "gpt-image-2",
		System:          "Never send this system prompt to image generation.",
		EnableTools:     true,
		ImageGeneration: true,
		ExtraParams:     map[string]any{"size": "2048x2048"},
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
	store.messages[matrixSessionID("!room:test", "image-maker")] = []persistence.ChatMessage{
		{Role: "user", Content: "previous prompt that must be ignored"},
		{Role: "assistant", Content: "previous answer that must be ignored"},
	}

	err = a.handleMatrixMessage(context.Background(), matrixgw.InboundMessage{
		RoomID:  "!room:test",
		EventID: "$event-image",
		Sender:  "@user:test",
		Body:    "@image-maker draw a river",
		Prompt:  "draw a river",
		Target:  "image-maker",
	})
	if err != nil {
		t.Fatalf("handleMatrixMessage() error = %v", err)
	}
	if gotPath != "/images/generations" {
		t.Fatalf("expected image generation endpoint, got %q", gotPath)
	}
	if model, _ := gotBody["model"].(string); model != "gpt-image-2" {
		t.Fatalf("expected gpt-image-2 model, got %#v", gotBody["model"])
	}
	if prompt, _ := gotBody["prompt"].(string); prompt != "draw a river" {
		t.Fatalf("expected Matrix prompt forwarded to image API, got %#v", gotBody["prompt"])
	}
	if size, _ := gotBody["size"].(string); size != "2048x2048" {
		t.Fatalf("expected configured Matrix image size, got %#v", gotBody["size"])
	}
	if len(client.uploaded) != 1 || string(client.uploaded[0]) != "pngbytes" {
		t.Fatalf("expected generated image upload, got %#v", client.uploaded)
	}
	if len(client.images) != 1 || client.images[0].URL != "mxc://matrix.example.com/uploaded" {
		t.Fatalf("expected Matrix image event, got %#v", client.images)
	}
}
