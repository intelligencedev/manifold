package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"manifold/internal/durable"
)

type durableEmitRequest struct {
	Queue   string         `json:"queue,omitempty"`
	Name    string         `json:"name"`
	Payload map[string]any `json:"payload,omitempty"`
}

type durableRetryRequest struct {
	ResetCheckpoints bool `json:"reset_checkpoints,omitempty"`
}

func (a *app) durableTasksHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.durableClient == nil || a.durableStore == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			filter, err := durableTaskListFilterFromQuery(r.URL.Query())
			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			tasks, err := a.durableClient.ListTasks(r.Context(), userID, filter)
			if err != nil {
				writeDurableError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
			return
		case http.MethodPost:
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()
		var req durable.SpawnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.UserID = userID
		if strings.TrimSpace(req.Queue) == "" {
			req.Queue = durable.DefaultQueue
		}
		if a.durableRegistry != nil {
			if _, ok := a.durableRegistry.Get(req.Queue, req.Name); !ok {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "durable handler not found"})
				return
			}
		}
		result, err := a.durableClient.Spawn(r.Context(), req)
		if err != nil {
			writeDurableError(w, err)
			return
		}
		status := http.StatusAccepted
		if result.Created {
			status = http.StatusCreated
		}
		writeJSON(w, status, result)
	}
}

func durableTaskListFilterFromQuery(values url.Values) (durable.TaskListFilter, error) {
	filter := durable.TaskListFilter{
		Queue: strings.TrimSpace(values.Get("queue")),
		Name:  strings.TrimSpace(values.Get("name")),
	}
	if status := strings.TrimSpace(values.Get("status")); status != "" {
		switch durable.TaskStatus(status) {
		case durable.TaskStatusQueued,
			durable.TaskStatusRunning,
			durable.TaskStatusWaiting,
			durable.TaskStatusCompleted,
			durable.TaskStatusFailed,
			durable.TaskStatusCancelled:
			filter.Status = durable.TaskStatus(status)
		default:
			return durable.TaskListFilter{}, errors.New("invalid status")
		}
	}
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 0 {
			return durable.TaskListFilter{}, errors.New("invalid limit")
		}
		filter.Limit = limit
	}
	return filter, nil
}

func (a *app) durableTaskDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.durableClient == nil || a.durableStore == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		taskID, suffix, ok := parseDurableTaskPath(r.URL.Path)
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
			task, found, err := a.durableStore.GetTask(r.Context(), userID, taskID)
			if err != nil {
				writeDurableError(w, err)
				return
			}
			if !found {
				http.Error(w, "task not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"task": task})
		case "events":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			a.serveDurableTaskEvents(w, r, userID, taskID)
		case "cancel":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := a.cancelDurableTask(r.Context(), userID, taskID); err != nil {
				writeDurableError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"task_id": taskID, "status": durable.TaskStatusCancelled})
		case "retry":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			req, ok := decodeDurableRetryRequest(w, r)
			if !ok {
				return
			}
			task, err := a.durableClient.Retry(r.Context(), userID, taskID, req.ResetCheckpoints)
			if err != nil {
				writeDurableError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"task": task})
		default:
			http.NotFound(w, r)
		}
	}
}

func decodeDurableRetryRequest(w http.ResponseWriter, r *http.Request) (durableRetryRequest, bool) {
	var req durableRetryRequest
	if r.Body == nil || r.ContentLength == 0 {
		return req, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return durableRetryRequest{}, false
	}
	return req, true
}

func (a *app) cancelDurableTask(ctx context.Context, userID int64, taskID string) error {
	if a == nil {
		return durable.ErrNotFound
	}
	if a.durableWorker != nil {
		return a.durableWorker.CancelTask(ctx, userID, taskID)
	}
	if a.durableClient != nil {
		return a.durableClient.Cancel(ctx, userID, taskID)
	}
	return durable.ErrNotFound
}

func (a *app) durableEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.durableClient == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
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
		var req durableEmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ev, err := a.durableClient.EmitEvent(r.Context(), userID, req.Queue, req.Name, req.Payload)
		if err != nil {
			writeDurableError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, ev)
	}
}

func (a *app) durableQueuesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.durableStore == nil {
			http.Error(w, "durable backend unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, err := a.requireUserID(r); err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stats, err := a.durableStore.QueueStats(r.Context())
		if err != nil {
			writeDurableError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"queues": stats})
	}
}

func (a *app) serveDurableTaskEvents(w http.ResponseWriter, r *http.Request, userID int64, taskID string) {
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		var seq int64
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			events, status, found, err := a.durableClient.ListEvents(r.Context(), userID, taskID, seq)
			if err != nil {
				writeDurableSSE(w, fl, map[string]any{"error": err.Error()})
				return
			}
			if !found {
				http.Error(w, "task not found", http.StatusNotFound)
				return
			}
			for _, ev := range events {
				if ev.Sequence > seq {
					seq = ev.Sequence
				}
				writeDurableSSE(w, fl, ev)
			}
			if durableTaskStatusTerminal(status) {
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
		}
	}
	events, status, found, err := a.durableClient.ListEvents(r.Context(), userID, taskID, 0)
	if err != nil {
		writeDurableError(w, err)
		return
	}
	if !found {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": taskID,
		"status":  status,
		"events":  events,
	})
}

func parseDurableTaskPath(path string) (string, string, bool) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/durable/tasks/"), "/")
	if trimmed == "" || trimmed == path {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) > 2 || parts[0] == "" {
		return "", "", false
	}
	taskID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(taskID) == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return taskID, "", true
	}
	return taskID, parts[1], true
}

func durableTaskStatusTerminal(status durable.TaskStatus) bool {
	return status == durable.TaskStatusCompleted || status == durable.TaskStatusFailed || status == durable.TaskStatusCancelled
}

func writeDurableSSE(w http.ResponseWriter, fl http.Flusher, payload any) {
	b, _ := json.Marshal(payload)
	_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
	fl.Flush()
}

func writeDurableError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, durable.ErrTaskNotFound), errors.Is(err, durable.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
	case errors.Is(err, durable.ErrInvalidState):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
}
