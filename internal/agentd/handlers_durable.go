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

	durableapi "manifold/internal/agentd/durableapi"
	"manifold/internal/durable"
)

type durableRetryRequest struct {
	ResetCheckpoints bool `json:"reset_checkpoints,omitempty"`
}

func (a *app) durableTasksHandler() http.HandlerFunc {
	return durableapi.TasksHandler(durableapi.Deps{
		Client: a.durableClient, Store: a.durableStore, Registry: a.durableRegistry, RequireUserID: a.requireUserID,
		AuthEnabled: func() bool { return a.cfg != nil && a.cfg.Auth.Enabled }, WriteJSON: writeJSON, WriteError: writeDurableError,
	})
}

func durableTaskListFilterFromQuery(values url.Values) (durable.TaskListFilter, error) {
	return durableapi.TaskListFilterFromQuery(values)
}

func durableTaskEventListFilterFromQuery(values url.Values) (durable.EventListFilter, error) {
	filter := durable.EventListFilter{Limit: durable.DefaultEventListLimit}
	if rawLimit := strings.TrimSpace(values.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return durable.EventListFilter{}, errors.New("invalid limit")
		}
		if limit > durable.MaxEventListLimit {
			limit = durable.MaxEventListLimit
		}
		filter.Limit = limit
	}
	after, hasAfter, err := parseOptionalSequence(values.Get("after"))
	if err != nil {
		return durable.EventListFilter{}, err
	}
	before, hasBefore, err := parseOptionalSequence(values.Get("before"))
	if err != nil {
		return durable.EventListFilter{}, err
	}
	if hasAfter && hasBefore {
		return durable.EventListFilter{}, errors.New("after and before are mutually exclusive")
	}
	filter.AfterSequence = after
	filter.BeforeSequence = before
	return filter, nil
}

func parseOptionalSequence(raw string) (int64, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq < 0 {
		return 0, false, errors.New("invalid sequence")
	}
	return seq, true, nil
}

func (a *app) durableTaskDetailHandler() http.HandlerFunc {
	return durableapi.TaskDetailHandler(durableapi.TaskDetailDeps{
		Deps: durableapi.Deps{Client: a.durableClient, Store: a.durableStore, Registry: a.durableRegistry, RequireUserID: a.requireUserID, AuthEnabled: func() bool { return a.cfg != nil && a.cfg.Auth.Enabled }, WriteJSON: writeJSON, WriteError: writeDurableError},
		GetTask: func(ctx context.Context, userID int64, taskID string) (durable.Task, bool, error) {
			if a.durableStore == nil {
				return durable.Task{}, false, durable.ErrNotFound
			}
			return a.durableStore.GetTask(ctx, userID, taskID)
		},
		ServeEvents: a.serveDurableTaskEvents,
		Cancel:      a.cancelDurableTask,
		Retry: func(ctx context.Context, userID int64, taskID string, resetCheckpoints bool) (durable.Task, error) {
			if a.durableClient == nil {
				return durable.Task{}, durable.ErrNotFound
			}
			return a.durableClient.Retry(ctx, userID, taskID, resetCheckpoints)
		},
	})
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
	return durableapi.EventsHandler(a.durableAPIHandlerDeps())
}

func (a *app) durableQueuesHandler() http.HandlerFunc {
	return durableapi.QueuesHandler(a.durableAPIHandlerDeps())
}

func (a *app) durableAPIHandlerDeps() durableapi.Deps {
	return durableapi.Deps{Client: a.durableClient, Store: a.durableStore, Registry: a.durableRegistry, RequireUserID: a.requireUserID, AuthEnabled: func() bool { return a.cfg != nil && a.cfg.Auth.Enabled }, WriteJSON: writeJSON, WriteError: writeDurableError}
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
	filter, err := durableTaskEventListFilterFromQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	page, err := a.durableClient.ListEventsPage(r.Context(), userID, taskID, filter)
	if err != nil {
		writeDurableError(w, err)
		return
	}
	if !page.Found {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id":         taskID,
		"status":          page.Status,
		"events":          page.Events,
		"limit":           page.Limit,
		"first_sequence":  page.FirstSequence,
		"last_sequence":   page.LastSequence,
		"has_more_before": page.HasMoreBefore,
		"has_more_after":  page.HasMoreAfter,
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
