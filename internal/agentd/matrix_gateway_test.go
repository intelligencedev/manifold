package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
	"manifold/internal/matrixgw"
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
