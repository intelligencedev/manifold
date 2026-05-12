package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/projects"
	"manifold/internal/pulse"
)

func TestMatrixRoomsHandlerListsConfiguredRoomsWithStats(t *testing.T) {
	t.Parallel()
	a := newSpecialistTestApp(t, "http://example.test", nil)
	a.cfg.Matrix = config.MatrixConfig{
		Enabled:          true,
		MessageRetention: 100,
		Rooms: []config.MatrixRoomConfig{{
			RoomID:           "!room:test",
			DefaultTarget:    "orchestrator",
			AllowUnmentioned: true,
			Mentions:         map[string]string{"@gpt": "openai"},
			MaxConcurrent:    2,
		}},
	}
	a.matrixMessageStore = databases.NewMatrixMessageStore(nil)
	pulseStore := databases.NewPulseStore(nil)
	if _, err := pulseStore.EnsureRoom(context.Background(), "!room:test", "openai"); err != nil {
		t.Fatalf("EnsureRoom() error = %v", err)
	}
	if _, err := pulseStore.UpsertTask(context.Background(), persistence.PulseTask{
		RoomID:          "!room:test",
		RouteTarget:     "openai",
		Title:           "Hourly check",
		Prompt:          "Post status",
		IntervalSeconds: 3600,
		Enabled:         true,
	}); err != nil {
		t.Fatalf("UpsertTask() error = %v", err)
	}
	a.pulseRuntime = newPulseRuntime(a, pulseStore)
	if _, err := a.matrixMessageStore.Append(context.Background(), persistence.MatrixMessage{
		RoomID:    "!room:test",
		Direction: "inbound",
		Sender:    "@user:test",
		Body:      "hello",
		MsgType:   "m.text",
	}, 100); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/matrix/rooms", nil)
	rr := httptest.NewRecorder()
	a.matrixRoomsHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Rooms []matrixRoomResponse `json:"rooms"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Rooms) != 1 {
		t.Fatalf("expected 1 room, got %#v", body.Rooms)
	}
	if body.Rooms[0].TaskCount != 1 || body.Rooms[0].Stats.MessageCount != 1 {
		t.Fatalf("unexpected room payload: %#v", body.Rooms[0])
	}
}

func TestMatrixTaskHandlersCreatePatchAndList(t *testing.T) {
	t.Parallel()
	a := newSpecialistTestApp(t, "http://example.test", nil)
	a.cfg.Matrix = config.MatrixConfig{
		Enabled: true,
		Rooms: []config.MatrixRoomConfig{{
			RoomID:        "!room:test",
			DefaultTarget: "orchestrator",
		}},
	}
	a.matrixMessageStore = databases.NewMatrixMessageStore(nil)
	pulseStore := databases.NewPulseStore(nil)
	a.pulseRuntime = newPulseRuntime(a, pulseStore)

	createReq := httptest.NewRequest(http.MethodPost, "/api/matrix/rooms/%21room%3Atest/tasks", strings.NewReader(`{"routeTarget":"openai","title":"Daily digest","prompt":"Summarize status","intervalSeconds":900,"enabled":true}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}
	var created matrixTaskResponse
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" || created.RouteTarget != "openai" {
		t.Fatalf("unexpected created task: %#v", created)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/matrix/rooms/%21room%3Atest/tasks/"+created.ID, strings.NewReader(`{"intervalSeconds":1800,"enabled":false}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(patchResp, patchReq)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patchResp.Code, patchResp.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/matrix/rooms/%21room%3Atest/tasks", nil)
	listResp := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResp.Code, listResp.Body.String())
	}
	var listed struct {
		Tasks []matrixTaskResponse `json:"tasks"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %#v", listed.Tasks)
	}
	if listed.Tasks[0].IntervalSeconds != 1800 || listed.Tasks[0].Enabled {
		t.Fatalf("expected patched task state, got %#v", listed.Tasks[0])
	}

	moveReq := httptest.NewRequest(http.MethodPatch, "/api/matrix/rooms/%21room%3Atest/tasks/"+created.ID, strings.NewReader(`{"routeTarget":"ops-team"}`))
	moveReq.Header.Set("Content-Type", "application/json")
	moveResp := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(moveResp, moveReq)
	if moveResp.Code != http.StatusOK {
		t.Fatalf("move status = %d, body = %s", moveResp.Code, moveResp.Body.String())
	}

	relistedResp := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(relistedResp, listReq)
	if relistedResp.Code != http.StatusOK {
		t.Fatalf("relist status = %d, body = %s", relistedResp.Code, relistedResp.Body.String())
	}
	var relisted struct {
		Tasks []matrixTaskResponse `json:"tasks"`
	}
	if err := json.NewDecoder(relistedResp.Body).Decode(&relisted); err != nil {
		t.Fatalf("decode relist response: %v", err)
	}
	if len(relisted.Tasks) != 1 || relisted.Tasks[0].RouteTarget != "ops-team" {
		t.Fatalf("expected moved task without duplication, got %#v", relisted.Tasks)
	}
}

func TestMatrixTaskHandlersSpecificSchedules(t *testing.T) {
	t.Parallel()
	a := newSpecialistTestApp(t, "http://example.test", nil)
	a.cfg.Matrix = config.MatrixConfig{
		Enabled: true,
		Rooms: []config.MatrixRoomConfig{{
			RoomID:        "!room:test",
			DefaultTarget: "orchestrator",
		}},
	}
	a.matrixMessageStore = databases.NewMatrixMessageStore(nil)
	a.pulseRuntime = newPulseRuntime(a, databases.NewPulseStore(nil))

	createReq := httptest.NewRequest(http.MethodPost, "/api/matrix/rooms/%21room%3Atest/tasks", strings.NewReader(`{"title":"Morning digest","prompt":"Summarize status","scheduleType":"daily_time","specificTime":"09:15","enabled":true}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}
	var created matrixTaskResponse
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ScheduleType != pulse.ScheduleDailyTime || created.SpecificTime != "09:15" || created.IntervalSeconds != 0 {
		t.Fatalf("unexpected daily schedule response: %#v", created)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/matrix/rooms/%21room%3Atest/tasks/"+created.ID, strings.NewReader(`{"scheduleType":"once_at","specificAt":"2026-05-12T09:00"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(patchResp, patchReq)
	if patchResp.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patchResp.Code, patchResp.Body.String())
	}
	var patched matrixTaskResponse
	if err := json.NewDecoder(patchResp.Body).Decode(&patched); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patched.ScheduleType != pulse.ScheduleOnceAt || patched.SpecificAt.IsZero() || patched.SpecificTime != "" {
		t.Fatalf("unexpected once_at schedule response: %#v", patched)
	}
}

func TestMatrixMessagesHandlerListsNewestFirst(t *testing.T) {
	t.Parallel()
	a := newSpecialistTestApp(t, "http://example.test", nil)
	a.cfg.Matrix = config.MatrixConfig{Enabled: true, Rooms: []config.MatrixRoomConfig{{RoomID: "!room:test", DefaultTarget: "orchestrator"}}}
	a.matrixMessageStore = databases.NewMatrixMessageStore(nil)
	a.pulseRuntime = newPulseRuntime(a, databases.NewPulseStore(nil))
	first, _ := a.matrixMessageStore.Append(context.Background(), persistence.MatrixMessage{RoomID: "!room:test", Direction: "inbound", Body: "first", MsgType: "m.text", CreatedAt: time.Now().UTC().Add(-time.Minute)}, 100)
	_, _ = a.matrixMessageStore.Append(context.Background(), persistence.MatrixMessage{RoomID: "!room:test", Direction: "outbound", Body: "second", MsgType: "m.text", CreatedAt: time.Now().UTC()}, 100)

	req := httptest.NewRequest(http.MethodGet, "/api/matrix/rooms/%21room%3Atest/messages?before="+strconv.FormatInt(first.ID+2, 10), nil)
	rr := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Messages []persistence.MatrixMessage `json:"messages"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Messages) != 2 || body.Messages[0].Body != "second" {
		t.Fatalf("unexpected messages payload: %#v", body.Messages)
	}
}

func TestMatrixSessionHandlerReturnsHiddenSession(t *testing.T) {
	t.Parallel()
	a := newSpecialistTestApp(t, "http://example.test", nil)
	a.cfg.Matrix = config.MatrixConfig{Enabled: true, Rooms: []config.MatrixRoomConfig{{RoomID: "!room:test", DefaultTarget: "orchestrator"}}}
	a.projectsService = projects.NewService(t.TempDir(), "")
	_, err := ensureMatrixChatSession(context.Background(), a.chatStore, "!room:test", matrixSessionID("!room:test"))
	if err != nil {
		t.Fatalf("ensureMatrixChatSession() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/matrix/rooms/%21room%3Atest/session", nil)
	rr := httptest.NewRecorder()
	a.matrixRoomDetailHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		SessionID string                  `json:"sessionId"`
		Session   persistence.ChatSession `json:"session"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.SessionID == "" || body.Session.Kind != persistence.ChatSessionKindMatrix {
		t.Fatalf("unexpected session payload: %#v", body)
	}
}
