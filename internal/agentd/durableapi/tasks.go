// Package durableapi owns the durable administration HTTP surface.
package durableapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"manifold/internal/durable"
)

type Deps struct {
	Client        *durable.Client
	Store         durable.Store
	Registry      *durable.Registry
	RequireUserID func(*http.Request) (int64, error)
	AuthEnabled   func() bool
	WriteJSON     func(http.ResponseWriter, int, any)
	WriteError    func(http.ResponseWriter, error)
}

type EmitRequest struct {
	Queue   string         `json:"queue,omitempty"`
	Name    string         `json:"name"`
	Payload map[string]any `json:"payload,omitempty"`
}

type TaskDetailDeps struct {
	Deps
	GetTask     func(context.Context, int64, string) (durable.Task, bool, error)
	ServeEvents func(http.ResponseWriter, *http.Request, int64, string)
	Cancel      func(context.Context, int64, string) error
	Retry       func(context.Context, int64, string, bool) (durable.Task, error)
}

// TasksHandler serves list and enqueue operations for durable tasks.
func TasksHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Client == nil || deps.Store == nil || deps.RequireUserID == nil || deps.WriteJSON == nil || deps.WriteError == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := deps.RequireUserID(r)
		if err != nil {
			if deps.AuthEnabled != nil && deps.AuthEnabled() {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			filter, err := TaskListFilterFromQuery(r.URL.Query())
			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			page, err := deps.Client.ListTasksPage(r.Context(), userID, filter)
			if err != nil {
				deps.WriteError(w, err)
				return
			}
			deps.WriteJSON(w, http.StatusOK, page)
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var request durable.SpawnRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			request.UserID = userID
			if strings.TrimSpace(request.Queue) == "" {
				request.Queue = durable.DefaultQueue
			}
			if deps.Registry != nil {
				if _, ok := deps.Registry.Get(request.Queue, request.Name); !ok {
					deps.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": "durable handler not found"})
					return
				}
			}
			result, err := deps.Client.Spawn(r.Context(), request)
			if err != nil {
				deps.WriteError(w, err)
				return
			}
			status := http.StatusAccepted
			if result.Created {
				status = http.StatusCreated
			}
			deps.WriteJSON(w, status, result)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// TaskListFilterFromQuery validates collection filters without depending on
// the application server.
func TaskListFilterFromQuery(values url.Values) (durable.TaskListFilter, error) {
	filter := durable.TaskListFilter{Queue: strings.TrimSpace(values.Get("queue")), Name: strings.TrimSpace(values.Get("name"))}
	if status := strings.TrimSpace(values.Get("status")); status != "" {
		switch durable.TaskStatus(status) {
		case durable.TaskStatusQueued, durable.TaskStatusRunning, durable.TaskStatusWaiting, durable.TaskStatusCompleted, durable.TaskStatusFailed, durable.TaskStatusCancelled:
			filter.Status = durable.TaskStatus(status)
		default:
			return durable.TaskListFilter{}, errors.New("invalid status")
		}
	}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return durable.TaskListFilter{}, errors.New("invalid limit")
		}
		filter.Limit = value
	}
	if raw := strings.TrimSpace(values.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return durable.TaskListFilter{}, errors.New("invalid offset")
		}
		filter.Offset = value
	}
	return filter, nil
}

// EventsHandler emits an application event to a durable task family.
func EventsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Client == nil || deps.RequireUserID == nil || deps.WriteJSON == nil || deps.WriteError == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := deps.RequireUserID(r)
		if err != nil {
			if deps.AuthEnabled != nil && deps.AuthEnabled() {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()
		var request EmitRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		event, err := deps.Client.EmitEvent(r.Context(), userID, request.Queue, request.Name, request.Payload)
		if err != nil {
			deps.WriteError(w, err)
			return
		}
		deps.WriteJSON(w, http.StatusAccepted, event)
	}
}

// QueuesHandler returns durable queue statistics.
func QueuesHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Store == nil || deps.RequireUserID == nil || deps.WriteJSON == nil || deps.WriteError == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, err := deps.RequireUserID(r); err != nil {
			if deps.AuthEnabled != nil && deps.AuthEnabled() {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stats, err := deps.Store.QueueStats(r.Context())
		if err != nil {
			deps.WriteError(w, err)
			return
		}
		deps.WriteJSON(w, http.StatusOK, map[string]any{"queues": stats})
	}
}

// TaskDetailHandler routes lookup, events, cancellation, and retry for one
// durable task. Worker lifecycle operations are supplied by composition-root
// callbacks, keeping this transport package independent of agentd.
func TaskDetailHandler(deps TaskDetailDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Client == nil || deps.Store == nil || deps.RequireUserID == nil || deps.WriteJSON == nil || deps.WriteError == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := deps.RequireUserID(r)
		if err != nil {
			if deps.AuthEnabled != nil && deps.AuthEnabled() {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		taskID, suffix, ok := parseTaskPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch suffix {
		case "":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			task, found, err := deps.GetTask(r.Context(), userID, taskID)
			if err != nil {
				deps.WriteError(w, err)
				return
			}
			if !found {
				http.Error(w, "task not found", http.StatusNotFound)
				return
			}
			deps.WriteJSON(w, http.StatusOK, map[string]any{"task": task})
		case "events":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			deps.ServeEvents(w, r, userID, taskID)
		case "cancel":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := deps.Cancel(r.Context(), userID, taskID); err != nil {
				deps.WriteError(w, err)
				return
			}
			deps.WriteJSON(w, http.StatusOK, map[string]any{"task_id": taskID, "status": durable.TaskStatusCancelled})
		case "retry":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			reset, ok := decodeRetryRequest(w, r)
			if !ok {
				return
			}
			task, err := deps.Retry(r.Context(), userID, taskID, reset)
			if err != nil {
				deps.WriteError(w, err)
				return
			}
			deps.WriteJSON(w, http.StatusOK, map[string]any{"task": task})
		default:
			http.NotFound(w, r)
		}
	}
}

func parseTaskPath(path string) (string, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/api/durable/tasks/"), "/")
	if rest == "" {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) > 2 {
		return "", "", false
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	return parts[0], parts[1], true
}

func decodeRetryRequest(w http.ResponseWriter, r *http.Request) (bool, bool) {
	if r.Body == nil || r.ContentLength == 0 {
		return false, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	var request struct {
		ResetCheckpoints bool `json:"reset_checkpoints,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return false, false
	}
	return request.ResetCheckpoints, true
}
