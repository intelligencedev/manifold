package agentd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/matrixgw"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
)

func TestPulseRuntimePollOnceRunsDueTaskWithoutPostingFinalReply(t *testing.T) {
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
	a.cfg.Matrix.Enabled = true
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

	store := databases.NewPulseStore(nil)
	ctx := context.Background()
	room, err := store.EnsureRoom(ctx, "!room:test", "weather")
	if err != nil {
		t.Fatalf("EnsureRoom() error = %v", err)
	}
	_, err = store.UpsertTask(ctx, persistence.PulseTask{
		RoomID:          room.RoomID,
		RouteTarget:     room.RouteTarget,
		Title:           "Check updates",
		Prompt:          "Review the latest updates.",
		IntervalSeconds: 60,
		Enabled:         true,
		LastRunAt:       time.Now().UTC().Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("UpsertTask() error = %v", err)
	}

	runtime := newPulseRuntime(a, store)
	runtime.interval = time.Minute
	runtime.lease = time.Minute
	if err := runtime.pollOnce(ctx); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}

	updatedRoom, err := store.GetRoom(ctx, room.RoomID, room.RouteTarget)
	if err != nil {
		t.Fatalf("GetRoom() error = %v", err)
	}
	if updatedRoom.LastPulseSummary != "specialist response" {
		t.Fatalf("expected stored pulse summary, got %q", updatedRoom.LastPulseSummary)
	}
	if updatedRoom.LastPulseError != "" {
		t.Fatalf("expected empty pulse error, got %q", updatedRoom.LastPulseError)
	}
	if len(client.sentHTML) != 0 || len(client.sentText) != 0 {
		t.Fatalf("expected no final Matrix send for pulse run, got html=%#v text=%#v", client.sentHTML, client.sentText)
	}

	chatStore := a.chatStore.(*promptHandlerChatStore)
	roomSessionID := matrixSessionID(room.RoomID)
	if _, ok := chatStore.sessions[roomSessionID]; !ok {
		t.Fatalf("expected room-scoped Matrix chat session %q to exist", roomSessionID)
	}
	storeMessages := chatStore.messages[roomSessionID]
	if len(storeMessages) < 2 {
		t.Fatalf("expected stored pulse chat turn in room-scoped Matrix session, got %#v", storeMessages)
	}
	if got := storeMessages[len(storeMessages)-1].Content; got != "specialist response" {
		t.Fatalf("expected stored pulse assistant response, got %q", got)
	}
}
