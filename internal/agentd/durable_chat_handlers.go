package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"manifold/internal/durable"
	"manifold/internal/fleet"
)

func (a *app) chatRunsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !a.durableChatAvailable() {
			http.Error(w, "durable chat backend unavailable", http.StatusServiceUnavailable)
			return
		}
		req, ok := prepareChatTransport(w, r, chatTransportOptions{})
		if !ok {
			return
		}
		state, ok := a.prepareChatHandlerState(w, r, req)
		if !ok {
			return
		}
		req = state.RunRequest
		a.ensureChatRunMessageIDs(&req)
		req.ObjectiveID = a.resolveChatObjectiveID(state.Request.Context(), state.Owner, req)
		target := resolveChatDispatchTarget(r.URL.Query())
		task, err := a.spawnDurableChatRun(state.Request.Context(), durableChatSpawnRequest{Request: req, Target: target, Endpoint: "/agent/run", Owner: state.Owner, UserID: state.UserID})
		if err != nil {
			writeDurableError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"run_id": task.ID, "session_id": req.SessionID, "user_message_id": req.UserMessageID, "assistant_message_id": req.AssistantMessageID, "status": task.Status})
	}
}

func (a *app) chatRunDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !a.durableChatAvailable() {
			http.Error(w, "durable chat backend unavailable", http.StatusServiceUnavailable)
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
		runID, suffix, requestID, ok := parseChatRunPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch suffix {
		case "events":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			after, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("after")), 10, 64)
			a.serveChatRunEvents(w, r, userID, runID, after)
		case "cancel":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := a.cancelDurableTask(r.Context(), userID, runID); err != nil {
				writeDurableError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "status": durable.TaskStatusCancelled})
		case "resume":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			task, retried, err := a.resumeOrAttachDurableChatRun(r.Context(), userID, runID)
			if err != nil {
				writeDurableError(w, err)
				return
			}
			sequences, err := a.durableChatSequenceSummary(r.Context(), userID, runID)
			if err != nil {
				writeDurableError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "status": task.Status, "retried": retried, "last_sequence": sequences.lastSequence, "last_retry_sequence": sequences.lastRetrySequence})
		case "input":
			if r.Method != http.MethodPost || requestID == "" {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			a.answerDurableChatInput(w, r, userID, runID, requestID)
		default:
			http.NotFound(w, r)
		}
	}
}

func parseChatRunPath(path string) (string, string, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(path, "/api/chat/runs/"), "/")
	if rest == "" || rest == path {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return "", "", "", false
	}
	runID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(runID) == "" {
		return "", "", "", false
	}
	switch parts[1] {
	case "events", "cancel", "resume":
		return runID, parts[1], "", len(parts) == 2
	case "input":
		if len(parts) != 4 || parts[3] != "answer" {
			return "", "", "", false
		}
		requestID, err := url.PathUnescape(parts[2])
		if err != nil || strings.TrimSpace(requestID) == "" {
			return "", "", "", false
		}
		return runID, "input", requestID, true
	default:
		return "", "", "", false
	}
}

func (a *app) answerDurableChatInput(w http.ResponseWriter, r *http.Request, userID int64, runID, requestID string) {
	task, found, err := a.durableStore.GetTask(r.Context(), userID, runID)
	if err != nil {
		writeDurableError(w, err)
		return
	}
	if !found {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	var body struct {
		Answer    string   `json:"answer"`
		ChoiceIDs []string `json:"choice_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	choiceIDs := make([]string, 0, len(body.ChoiceIDs))
	for _, id := range body.ChoiceIDs {
		if id = strings.TrimSpace(id); id != "" {
			choiceIDs = append(choiceIDs, id)
		}
	}
	payload := map[string]any{"request_id": requestID, "answer": strings.TrimSpace(body.Answer), "choice_ids": choiceIDs, "responded_at": time.Now().UTC()}
	if _, err := a.durableClient.EmitEvent(r.Context(), userID, durableChatQueue, durableChatInputAnswerEvent(requestID), payload); err != nil {
		writeDurableError(w, err)
		return
	}
	if a.fleetBus != nil {
		metadata := durableChatTaskMetadataFromTask(task)
		a.fleetBus.Publish(fleet.Event{Kind: fleet.EventInputAnswered, RunID: runID, SessionID: metadata.sessionID, UserID: userID, Data: map[string]any{"request_id": requestID, "answer": strings.TrimSpace(body.Answer), "choice_ids": choiceIDs}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "request_id": requestID})
}

func (a *app) serveChatRunEvents(w http.ResponseWriter, r *http.Request, userID int64, runID string, after int64) {
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		seq := after
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			events, status, found, err := a.durableClient.ListEvents(r.Context(), userID, runID, seq)
			if err != nil {
				writeDurableSSE(w, fl, map[string]any{"type": "error", "data": err.Error(), "run_id": runID})
				return
			}
			if !found {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			for _, event := range events {
				if event.Sequence > seq {
					seq = event.Sequence
				}
				writeDurableSSE(w, fl, durableChatStreamPayload(runID, event))
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
	events, status, found, err := a.durableClient.ListEvents(r.Context(), userID, runID, after)
	if err != nil {
		writeDurableError(w, err)
		return
	}
	if !found {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	payloads := make([]map[string]any, 0, len(events))
	for _, event := range events {
		payloads = append(payloads, durableChatStreamPayload(runID, event))
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "status": status, "events": payloads})
}

func (a *app) handleChatSessionRuns(w http.ResponseWriter, r *http.Request, userID *int64, sessionID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.durableChatAvailable() {
		http.Error(w, "durable chat backend unavailable", http.StatusServiceUnavailable)
		return
	}
	if _, err := a.chatStore.GetSession(r.Context(), userID, sessionID); err != nil {
		writeChatDetailStoreError(w, r, err, sessionID, "get_chat_session_for_runs")
		return
	}
	durableUserID := systemUserID
	if userID != nil {
		durableUserID = *userID
	}
	tasks, err := a.durableClient.ListTasks(r.Context(), durableUserID, durable.TaskListFilter{Queue: durableChatQueue, Name: durableChatRunTaskName, Limit: 200})
	if err != nil {
		writeDurableError(w, err)
		return
	}
	activeOnly := isTruthyQueryValue(r.URL.Query().Get("active"))
	runs := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		metadata := durableChatTaskMetadataFromTask(task)
		if metadata.sessionID != sessionID || (activeOnly && !durableChatRecoverableStatus(task.Status)) {
			continue
		}
		sequences, err := a.durableChatSequenceSummary(r.Context(), durableUserID, task.ID)
		if err != nil {
			writeDurableError(w, err)
			return
		}
		runs = append(runs, map[string]any{"run_id": task.ID, "status": task.Status, "session_id": metadata.sessionID, "user_message_id": metadata.userMessageID, "assistant_message_id": metadata.assistantMessageID, "created_at": task.CreatedAt, "updated_at": task.UpdatedAt, "error": task.Error, "last_sequence": sequences.lastSequence, "last_retry_sequence": sequences.lastRetrySequence})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

type durableChatSequenceSummary struct {
	lastSequence      int64
	lastRetrySequence int64
}

func (a *app) durableChatSequenceSummary(ctx context.Context, userID int64, runID string) (durableChatSequenceSummary, error) {
	events, _, found, err := a.durableClient.ListEvents(ctx, userID, runID, 0)
	if err != nil {
		return durableChatSequenceSummary{}, err
	}
	if !found {
		return durableChatSequenceSummary{}, durable.ErrTaskNotFound
	}
	var summary durableChatSequenceSummary
	for _, event := range events {
		if event.Sequence > summary.lastSequence {
			summary.lastSequence = event.Sequence
		}
		if event.Name == "task_retried" && event.Sequence > summary.lastRetrySequence {
			summary.lastRetrySequence = event.Sequence
		}
	}
	return summary, nil
}

func (a *app) resumeOrAttachDurableChatRun(ctx context.Context, userID int64, runID string) (durable.Task, bool, error) {
	if a == nil || a.durableStore == nil || a.durableClient == nil {
		return durable.Task{}, false, durable.ErrNotFound
	}
	task, found, err := a.durableStore.GetTask(ctx, userID, runID)
	if err != nil {
		return durable.Task{}, false, err
	}
	if !found {
		return durable.Task{}, false, durable.ErrTaskNotFound
	}
	switch task.Status {
	case durable.TaskStatusFailed, durable.TaskStatusCancelled:
		retried, err := a.durableClient.Retry(ctx, userID, runID, false)
		return retried, true, err
	default:
		return task, false, nil
	}
}

func durableChatRecoverableStatus(status durable.TaskStatus) bool {
	return status == durable.TaskStatusQueued || status == durable.TaskStatusRunning || status == durable.TaskStatusWaiting || status == durable.TaskStatusFailed
}

func durableChatTaskMetadataFromTask(task durable.Task) durableChatTaskMetadata {
	params, err := durableChatTaskParamsFromMap(task.Params)
	if err != nil {
		return durableChatTaskMetadata{}
	}
	return durableChatTaskMetadata{sessionID: params.Request.SessionID, userMessageID: params.Request.UserMessageID, assistantMessageID: params.Request.AssistantMessageID}
}

func (a *app) tryHandleDurableChatCompatibility(w http.ResponseWriter, r *http.Request, req chatRunRequest, state *preparedChatHandlerState, target chatDispatchTarget, endpoint string) bool {
	if !a.durableChatAvailable() || state == nil {
		return false
	}
	a.ensureChatRunMessageIDs(&req)
	task, err := a.spawnDurableChatRun(context.WithoutCancel(state.Request.Context()), durableChatSpawnRequest{Request: req, Target: target, Endpoint: endpoint, Owner: state.Owner, UserID: state.UserID})
	if err != nil {
		writeDurableError(w, err)
		return true
	}
	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		userID := systemUserID
		if state.UserID != nil {
			userID = *state.UserID
		}
		a.serveChatRunEvents(w, r, userID, task.ID, 0)
		return true
	}
	userID := systemUserID
	if state.UserID != nil {
		userID = *state.UserID
	}
	snapshot, err := a.durableClient.AwaitResult(r.Context(), userID, task.ID, 250*time.Millisecond)
	if err != nil {
		writeDurableError(w, err)
		return true
	}
	if snapshot.State != durable.TaskStatusCompleted {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": snapshot.Error, "run_id": task.ID, "status": snapshot.State})
		return true
	}
	var result map[string]any
	if err := snapshot.DecodeResult(&result); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "run_id": task.ID})
		return true
	}
	writeJSON(w, http.StatusOK, result)
	return true
}
