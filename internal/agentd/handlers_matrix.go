package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"manifold/internal/persistence"
	"manifold/internal/pulse"
)

type matrixRouteStateResponse struct {
	RouteTarget          string    `json:"routeTarget"`
	ProjectID            string    `json:"projectId,omitempty"`
	Enabled              bool      `json:"enabled"`
	Revision             int64     `json:"revision"`
	ActiveClaimToken     string    `json:"activeClaimToken,omitempty"`
	ActiveClaimUntil     time.Time `json:"activeClaimUntil"`
	LastPulseAttemptAt   time.Time `json:"lastPulseAttemptAt"`
	LastPulseCompletedAt time.Time `json:"lastPulseCompletedAt"`
	LastPulseSummary     string    `json:"lastPulseSummary,omitempty"`
	LastPulseError       string    `json:"lastPulseError,omitempty"`
}

type matrixRoomResponse struct {
	RoomID           string                      `json:"roomId"`
	DefaultTarget    string                      `json:"defaultTarget"`
	AllowUnmentioned bool                        `json:"allowUnmentioned"`
	Mentions         map[string]string           `json:"mentions"`
	SystemPromptRef  string                      `json:"systemPromptRef,omitempty"`
	MaxConcurrent    int                         `json:"maxConcurrent"`
	MessageRetention int                         `json:"messageRetention"`
	SessionID        string                      `json:"sessionId"`
	Stats            persistence.MatrixRoomStats `json:"stats"`
	Routes           []matrixRouteStateResponse  `json:"routes"`
	TaskCount        int                         `json:"taskCount"`
	EnabledTaskCount int                         `json:"enabledTaskCount"`
}

type matrixTaskResponse struct {
	ID                string    `json:"id"`
	RoomID            string    `json:"roomId"`
	RouteTarget       string    `json:"routeTarget,omitempty"`
	Title             string    `json:"title"`
	Prompt            string    `json:"prompt"`
	ScheduleType      string    `json:"scheduleType"`
	ScheduleLabel     string    `json:"scheduleLabel"`
	IntervalSeconds   int       `json:"intervalSeconds"`
	IntervalHuman     string    `json:"intervalHuman"`
	SpecificTime      string    `json:"specificTime,omitempty"`
	SpecificAt        time.Time `json:"specificAt"`
	Enabled           bool      `json:"enabled"`
	RoomEnabled       bool      `json:"roomEnabled"`
	Due               bool      `json:"due"`
	LastRunAt         time.Time `json:"lastRunAt"`
	LastRunHuman      string    `json:"lastRunHuman,omitempty"`
	NextRunAt         time.Time `json:"nextRunAt"`
	NextRunHuman      string    `json:"nextRunHuman,omitempty"`
	LastResultSummary string    `json:"lastResultSummary,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type matrixTaskUpsertRequest struct {
	RouteTarget     string `json:"routeTarget"`
	Title           string `json:"title"`
	Prompt          string `json:"prompt"`
	ScheduleType    string `json:"scheduleType"`
	IntervalSeconds int    `json:"intervalSeconds"`
	SpecificTime    string `json:"specificTime"`
	SpecificAt      string `json:"specificAt"`
	Enabled         *bool  `json:"enabled"`
}

type matrixTaskPatchRequest struct {
	RouteTarget     *string `json:"routeTarget"`
	Title           *string `json:"title"`
	Prompt          *string `json:"prompt"`
	ScheduleType    *string `json:"scheduleType"`
	IntervalSeconds *int    `json:"intervalSeconds"`
	SpecificTime    *string `json:"specificTime"`
	SpecificAt      *string `json:"specificAt"`
	Enabled         *bool   `json:"enabled"`
}

func (a *app) matrixRoomsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok, err := a.resolveProjectsUser(r); !ok || err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		rooms, err := a.matrixRoomResponses(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"rooms": rooms})
	}
}

func (a *app) matrixRoomDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if _, ok, err := a.resolveProjectsUser(r); !ok || err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		roomID, rest, ok := matrixRoomPath(strings.TrimPrefix(r.URL.Path, "/api/matrix/rooms/"))
		if !ok {
			http.NotFound(w, r)
			return
		}

		switch {
		case rest == "messages":
			a.handleMatrixRoomMessages(w, r, roomID)
		case rest == "tasks":
			a.handleMatrixRoomTasks(w, r, roomID)
		case strings.HasPrefix(rest, "tasks/"):
			a.handleMatrixRoomTaskDetail(w, r, roomID, strings.TrimPrefix(rest, "tasks/"))
		case rest == "session":
			a.handleMatrixRoomSession(w, r, roomID)
		default:
			http.NotFound(w, r)
		}
	}
}

func (a *app) handleMatrixRoomMessages(w http.ResponseWriter, r *http.Request, roomID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	var beforeID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		beforeID = parsed
	}
	messages, err := a.matrixMessageStore.ListByRoom(r.Context(), roomID, limit, beforeID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"messages": messages})
}

func (a *app) handleMatrixRoomTasks(w http.ResponseWriter, r *http.Request, roomID string) {
	switch r.Method {
	case http.MethodGet:
		tasks, err := a.matrixTaskResponses(r.Context(), roomID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tasks": tasks})
	case http.MethodPost:
		var req matrixTaskUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		task, err := a.upsertMatrixTask(r.Context(), roomID, "", req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(task)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) handleMatrixRoomTaskDetail(w http.ResponseWriter, r *http.Request, roomID, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	taskID, err := url.PathUnescape(parts[0])
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "run-now":
			task, err := a.forceMatrixTaskRunNow(r.Context(), roomID, taskID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(task)
			return
		case "enable":
			enabled := true
			task, err := a.patchMatrixTaskEnabled(r.Context(), roomID, taskID, &enabled)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(task)
			return
		case "disable":
			enabled := false
			task, err := a.patchMatrixTaskEnabled(r.Context(), roomID, taskID, &enabled)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(task)
			return
		}
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req matrixTaskPatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		task, err := a.patchMatrixTask(r.Context(), roomID, taskID, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(task)
	case http.MethodDelete:
		if err := a.deleteMatrixTask(r.Context(), roomID, taskID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) handleMatrixRoomSession(w http.ResponseWriter, r *http.Request, roomID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := matrixSessionID(roomID)
	session, err := a.chatStore.GetSession(r.Context(), nil, sessionID)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sessionId": sessionID,
		"session":   session,
	})
}

func (a *app) matrixRoomResponses(ctx context.Context) ([]matrixRoomResponse, error) {
	routeStates, err := a.matrixRouteStates(ctx)
	if err != nil {
		return nil, err
	}
	responses := make([]matrixRoomResponse, 0, len(a.cfg.Matrix.Rooms))
	for _, room := range a.cfg.Matrix.Rooms {
		stats, err := a.matrixMessageStore.RoomStats(ctx, room.RoomID)
		if err != nil {
			return nil, err
		}
		routes := routeStates[room.RoomID]
		taskCount := 0
		enabledTaskCount := 0
		for _, route := range routes {
			tasks, err := a.pulseRuntime.store.ListTasks(ctx, room.RoomID, route.RouteTarget)
			if err != nil {
				return nil, err
			}
			taskCount += len(tasks)
			for _, task := range tasks {
				if task.Enabled {
					enabledTaskCount++
				}
			}
		}
		responses = append(responses, matrixRoomResponse{
			RoomID:           room.RoomID,
			DefaultTarget:    room.DefaultTarget,
			AllowUnmentioned: room.AllowUnmentioned,
			Mentions:         room.Mentions,
			SystemPromptRef:  room.SystemPromptRef,
			MaxConcurrent:    room.MaxConcurrent,
			MessageRetention: matrixMessageRetention(a.cfg.Matrix, room.RoomID),
			SessionID:        matrixSessionID(room.RoomID),
			Stats:            stats,
			Routes:           routes,
			TaskCount:        taskCount,
			EnabledTaskCount: enabledTaskCount,
		})
	}
	return responses, nil
}

func (a *app) matrixRouteStates(ctx context.Context) (map[string][]matrixRouteStateResponse, error) {
	out := map[string][]matrixRouteStateResponse{}
	if a.pulseRuntime == nil || a.pulseRuntime.store == nil {
		return out, nil
	}
	rooms, err := a.pulseRuntime.store.ListRooms(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, room := range rooms {
		out[room.RoomID] = append(out[room.RoomID], matrixRouteStateResponse{
			RouteTarget:          room.RouteTarget,
			ProjectID:            room.ProjectID,
			Enabled:              room.Enabled,
			Revision:             room.Revision,
			ActiveClaimToken:     room.ActiveClaimToken,
			ActiveClaimUntil:     room.ActiveClaimUntil,
			LastPulseAttemptAt:   room.LastPulseAttemptAt,
			LastPulseCompletedAt: room.LastPulseCompletedAt,
			LastPulseSummary:     room.LastPulseSummary,
			LastPulseError:       room.LastPulseError,
		})
	}
	for roomID := range out {
		sort.Slice(out[roomID], func(i, j int) bool {
			return out[roomID][i].RouteTarget < out[roomID][j].RouteTarget
		})
	}
	return out, nil
}

func (a *app) matrixTaskResponses(ctx context.Context, roomID string) ([]matrixTaskResponse, error) {
	if a.pulseRuntime == nil || a.pulseRuntime.store == nil {
		return []matrixTaskResponse{}, nil
	}
	rooms, err := a.pulseRuntime.store.ListRooms(ctx, "")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	planner := pulse.NewService()
	out := make([]matrixTaskResponse, 0)
	for _, room := range rooms {
		if room.RoomID != roomID {
			continue
		}
		tasks, err := a.pulseRuntime.store.ListTasks(ctx, room.RoomID, room.RouteTarget)
		if err != nil {
			return nil, err
		}
		plan := planner.EvaluateRoom(now, room, tasks, room.RouteTarget)
		for _, status := range plan.Tasks {
			response := matrixTaskResponse{
				ID:                status.Task.ID,
				RoomID:            status.Task.RoomID,
				RouteTarget:       status.Task.RouteTarget,
				Title:             status.Task.Title,
				Prompt:            status.Task.Prompt,
				ScheduleType:      status.Task.ScheduleType,
				ScheduleLabel:     status.ScheduleLabel,
				IntervalSeconds:   status.Task.IntervalSeconds,
				IntervalHuman:     status.IntervalHuman,
				SpecificTime:      status.Task.SpecificTime,
				SpecificAt:        status.Task.SpecificAt,
				Enabled:           status.Task.Enabled,
				RoomEnabled:       room.Enabled,
				Due:               status.Due,
				LastRunAt:         status.Task.LastRunAt,
				LastRunHuman:      status.LastRunHuman,
				LastResultSummary: status.Task.LastResultSummary,
				CreatedAt:         status.Task.CreatedAt,
				UpdatedAt:         status.Task.UpdatedAt,
			}
			if !status.NextRunAt.IsZero() {
				response.NextRunAt = status.NextRunAt
				response.NextRunHuman = status.NextRunAt.Format(time.RFC3339)
			}
			out = append(out, response)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Due != out[j].Due {
			return out[i].Due
		}
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled
		}
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Title < out[j].Title
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if out == nil {
		out = []matrixTaskResponse{}
	}
	return out, nil
}

func (a *app) matrixRoomConfig(roomID string) (configRoom matrixRoomResponse, ok bool) {
	for _, room := range a.cfg.Matrix.Rooms {
		if room.RoomID != roomID {
			continue
		}
		return matrixRoomResponse{
			RoomID:           room.RoomID,
			DefaultTarget:    room.DefaultTarget,
			AllowUnmentioned: room.AllowUnmentioned,
			Mentions:         room.Mentions,
			SystemPromptRef:  room.SystemPromptRef,
			MaxConcurrent:    room.MaxConcurrent,
			MessageRetention: matrixMessageRetention(a.cfg.Matrix, room.RoomID),
			SessionID:        matrixSessionID(room.RoomID),
		}, true
	}
	return matrixRoomResponse{}, false
}

func matrixRoomPath(path string) (roomID, rest string, ok bool) {
	path = strings.Trim(path, "/")
	if path == "" {
		return "", "", false
	}
	parts := strings.SplitN(path, "/", 2)
	roomID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(roomID) == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return roomID, "", true
	}
	return roomID, parts[1], true
}
