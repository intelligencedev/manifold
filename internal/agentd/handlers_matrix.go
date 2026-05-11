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
	ActiveClaimUntil     time.Time `json:"activeClaimUntil,omitempty"`
	LastPulseAttemptAt   time.Time `json:"lastPulseAttemptAt,omitempty"`
	LastPulseCompletedAt time.Time `json:"lastPulseCompletedAt,omitempty"`
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
	IntervalSeconds   int       `json:"intervalSeconds"`
	IntervalHuman     string    `json:"intervalHuman"`
	Enabled           bool      `json:"enabled"`
	RoomEnabled       bool      `json:"roomEnabled"`
	Due               bool      `json:"due"`
	LastRunAt         time.Time `json:"lastRunAt,omitempty"`
	LastRunHuman      string    `json:"lastRunHuman,omitempty"`
	NextRunAt         time.Time `json:"nextRunAt,omitempty"`
	NextRunHuman      string    `json:"nextRunHuman,omitempty"`
	LastResultSummary string    `json:"lastResultSummary,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
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
		var req struct {
			RouteTarget     string `json:"routeTarget"`
			Title           string `json:"title"`
			Prompt          string `json:"prompt"`
			IntervalSeconds int    `json:"intervalSeconds"`
			Enabled         *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		task, err := a.upsertMatrixTask(r.Context(), roomID, "", req.RouteTarget, req.Title, req.Prompt, req.IntervalSeconds, req.Enabled)
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
		var req struct {
			RouteTarget     *string `json:"routeTarget"`
			Title           *string `json:"title"`
			Prompt          *string `json:"prompt"`
			IntervalSeconds *int    `json:"intervalSeconds"`
			Enabled         *bool   `json:"enabled"`
		}
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
				IntervalSeconds:   status.Task.IntervalSeconds,
				IntervalHuman:     status.IntervalHuman,
				Enabled:           status.Task.Enabled,
				RoomEnabled:       room.Enabled,
				Due:               status.Due,
				LastRunAt:         status.Task.LastRunAt,
				LastRunHuman:      status.LastRunHuman,
				LastResultSummary: status.Task.LastResultSummary,
				CreatedAt:         status.Task.CreatedAt,
				UpdatedAt:         status.Task.UpdatedAt,
			}
			if nextRunAt, ok := matrixTaskNextRunAt(now, room.Enabled, status.Task); ok {
				response.NextRunAt = nextRunAt
				response.NextRunHuman = nextRunAt.Format(time.RFC3339)
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

func matrixTaskNextRunAt(now time.Time, roomEnabled bool, task persistence.PulseTask) (time.Time, bool) {
	if !roomEnabled || !task.Enabled {
		return time.Time{}, false
	}
	interval := time.Duration(task.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if task.LastRunAt.IsZero() {
		return now, true
	}
	next := task.LastRunAt.UTC().Add(interval)
	if next.Before(now) {
		return now, true
	}
	return next, true
}

func (a *app) upsertMatrixTask(ctx context.Context, roomID, taskID, routeTarget, title, prompt string, intervalSeconds int, enabled *bool) (matrixTaskResponse, error) {
	roomConfig, ok := a.matrixRoomConfig(roomID)
	if !ok {
		return matrixTaskResponse{}, errors.New("matrix room not configured")
	}
	resolvedTarget := strings.TrimSpace(routeTarget)
	if resolvedTarget == "" {
		resolvedTarget = strings.TrimSpace(roomConfig.DefaultTarget)
	}
	if resolvedTarget == "" {
		return matrixTaskResponse{}, errors.New("route target required")
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(prompt) == "" {
		return matrixTaskResponse{}, errors.New("title and prompt are required")
	}
	if intervalSeconds <= 0 {
		intervalSeconds = 300
	}
	room, err := a.pulseRuntime.store.EnsureRoom(ctx, roomID, resolvedTarget)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	task := persistence.PulseTask{
		ID:              strings.TrimSpace(taskID),
		RoomID:          roomID,
		RouteTarget:     resolvedTarget,
		Title:           strings.TrimSpace(title),
		Prompt:          strings.TrimSpace(prompt),
		IntervalSeconds: intervalSeconds,
		Enabled:         room.Enabled,
	}
	if enabled != nil {
		task.Enabled = *enabled
	}
	stored, err := a.pulseRuntime.store.UpsertTask(ctx, task)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	return a.matrixTaskResponseFor(ctx, room, stored)
}

func (a *app) patchMatrixTask(ctx context.Context, roomID, taskID string, req struct {
	RouteTarget     *string `json:"routeTarget"`
	Title           *string `json:"title"`
	Prompt          *string `json:"prompt"`
	IntervalSeconds *int    `json:"intervalSeconds"`
	Enabled         *bool   `json:"enabled"`
}) (matrixTaskResponse, error) {
	room, task, err := a.matrixTaskByID(ctx, roomID, taskID)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	if req.RouteTarget != nil {
		task.RouteTarget = strings.TrimSpace(*req.RouteTarget)
	}
	if req.Title != nil {
		task.Title = strings.TrimSpace(*req.Title)
	}
	if req.Prompt != nil {
		task.Prompt = strings.TrimSpace(*req.Prompt)
	}
	if req.IntervalSeconds != nil {
		task.IntervalSeconds = *req.IntervalSeconds
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if task.RouteTarget == "" {
		task.RouteTarget = room.RouteTarget
	}
	previousRouteTarget := room.RouteTarget
	stored, err := a.pulseRuntime.store.UpsertTask(ctx, task)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	if previousRouteTarget != "" && previousRouteTarget != stored.RouteTarget {
		if err := a.pulseRuntime.store.DeleteTask(ctx, roomID, previousRouteTarget, stored.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return matrixTaskResponse{}, err
		}
	}
	updatedRoom, err := a.pulseRuntime.store.GetRoom(ctx, roomID, stored.RouteTarget)
	if err != nil {
		updatedRoom = room
	}
	return a.matrixTaskResponseFor(ctx, updatedRoom, stored)
}

func (a *app) patchMatrixTaskEnabled(ctx context.Context, roomID, taskID string, enabled *bool) (matrixTaskResponse, error) {
	return a.patchMatrixTask(ctx, roomID, taskID, struct {
		RouteTarget     *string `json:"routeTarget"`
		Title           *string `json:"title"`
		Prompt          *string `json:"prompt"`
		IntervalSeconds *int    `json:"intervalSeconds"`
		Enabled         *bool   `json:"enabled"`
	}{Enabled: enabled})
}

func (a *app) forceMatrixTaskRunNow(ctx context.Context, roomID, taskID string) (matrixTaskResponse, error) {
	room, task, err := a.matrixTaskByID(ctx, roomID, taskID)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	interval := time.Duration(task.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	task.LastRunAt = time.Now().UTC().Add(-interval - time.Second)
	stored, err := a.pulseRuntime.store.UpsertTask(ctx, task)
	if err != nil {
		return matrixTaskResponse{}, err
	}
	return a.matrixTaskResponseFor(ctx, room, stored)
}

func (a *app) deleteMatrixTask(ctx context.Context, roomID, taskID string) error {
	room, task, err := a.matrixTaskByID(ctx, roomID, taskID)
	if err != nil {
		return err
	}
	return a.pulseRuntime.store.DeleteTask(ctx, room.RoomID, room.RouteTarget, task.ID)
}

func (a *app) matrixTaskByID(ctx context.Context, roomID, taskID string) (persistence.PulseRoom, persistence.PulseTask, error) {
	rooms, err := a.pulseRuntime.store.ListRooms(ctx, "")
	if err != nil {
		return persistence.PulseRoom{}, persistence.PulseTask{}, err
	}
	for _, room := range rooms {
		if room.RoomID != roomID {
			continue
		}
		tasks, err := a.pulseRuntime.store.ListTasks(ctx, room.RoomID, room.RouteTarget)
		if err != nil {
			return persistence.PulseRoom{}, persistence.PulseTask{}, err
		}
		for _, task := range tasks {
			if task.ID == taskID {
				return room, task, nil
			}
		}
	}
	return persistence.PulseRoom{}, persistence.PulseTask{}, persistence.ErrNotFound
}

func (a *app) matrixTaskResponseFor(ctx context.Context, room persistence.PulseRoom, task persistence.PulseTask) (matrixTaskResponse, error) {
	planner := pulse.NewService()
	plan := planner.EvaluateRoom(time.Now().UTC(), room, []persistence.PulseTask{task}, room.RouteTarget)
	if len(plan.Tasks) == 0 {
		return matrixTaskResponse{}, persistence.ErrNotFound
	}
	status := plan.Tasks[0]
	response := matrixTaskResponse{
		ID:                status.Task.ID,
		RoomID:            status.Task.RoomID,
		RouteTarget:       status.Task.RouteTarget,
		Title:             status.Task.Title,
		Prompt:            status.Task.Prompt,
		IntervalSeconds:   status.Task.IntervalSeconds,
		IntervalHuman:     status.IntervalHuman,
		Enabled:           status.Task.Enabled,
		RoomEnabled:       room.Enabled,
		Due:               status.Due,
		LastRunAt:         status.Task.LastRunAt,
		LastRunHuman:      status.LastRunHuman,
		LastResultSummary: status.Task.LastResultSummary,
		CreatedAt:         status.Task.CreatedAt,
		UpdatedAt:         status.Task.UpdatedAt,
	}
	if nextRunAt, ok := matrixTaskNextRunAt(time.Now().UTC(), room.Enabled, status.Task); ok {
		response.NextRunAt = nextRunAt
		response.NextRunHuman = nextRunAt.Format(time.RFC3339)
	}
	return response, nil
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
