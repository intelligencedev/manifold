package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"manifold/internal/durable"
	"manifold/internal/warpp"
	"manifold/internal/warpp/toolnode"
)

type warppGetResponse struct {
	Document warpp.Document `json:"document"`
	Canvas   warpp.Canvas   `json:"canvas"`
}

type warppPutRequest struct {
	Document warpp.Document `json:"document"`
	Canvas   warpp.Canvas   `json:"canvas"`
}

type warppValidateRequest struct {
	Document warpp.Document `json:"document"`
}

type warppValidateResponse struct {
	Valid       bool               `json:"valid"`
	Diagnostics []warpp.Diagnostic `json:"diagnostics,omitempty"`
}

type warppRunHTTPRequest struct {
	WorkflowID string         `json:"workflow_id"`
	Input      map[string]any `json:"input,omitempty"`
	ProjectID  string         `json:"project_id,omitempty"`
}

type warppRunHTTPResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type warppCatalogResponse struct {
	Manifests []warpp.Manifest       `json:"manifests"`
	Coercions [][2]string            `json:"coercions"`
	Workflows []warppWorkflowSummary `json:"workflows"`
}

func (a *app) requireWarppUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, err := a.requireUserID(r)
	if err != nil {
		if a.cfg != nil && a.cfg.Auth.Enabled {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	return userID, true
}

func (a *app) warppWorkflowsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireWarppUser(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		workflows, err := a.warppState().listWorkflowSummaries(r.Context(), userID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeWarppJSON(w, http.StatusOK, map[string]any{"workflows": workflows})
	}
}

func (a *app) warppWorkflowDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireWarppUser(w, r)
		if !ok {
			return
		}
		workflowID, ok := warppWorkflowID(w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			a.getWarppWorkflow(w, r, userID, workflowID)
		case http.MethodPut:
			a.putWarppWorkflow(w, r, userID, workflowID)
		case http.MethodDelete:
			a.deleteWarppWorkflow(w, r, userID, workflowID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func warppWorkflowID(w http.ResponseWriter, r *http.Request) (string, bool) {
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/api/warpp/workflows/")
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		http.NotFound(w, r)
		return "", false
	}
	id, err := url.PathUnescape(path)
	if err != nil {
		http.Error(w, "bad workflow id", http.StatusBadRequest)
		return "", false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return "", false
	}
	return id, true
}

func (a *app) getWarppWorkflow(w http.ResponseWriter, r *http.Request, userID int64, workflowID string) {
	doc, canvas, found, err := a.warppState().getWorkflow(r.Context(), userID, workflowID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	writeWarppJSON(w, http.StatusOK, warppGetResponse{Document: doc, Canvas: canvas})
}

func (a *app) putWarppWorkflow(w http.ResponseWriter, r *http.Request, userID int64, workflowID string) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	var req warppPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Document.ID) == "" {
		req.Document.ID = workflowID
	}
	if req.Document.ID != workflowID {
		http.Error(w, "workflow id mismatch", http.StatusBadRequest)
		return
	}
	diags := warpp.Validate(req.Document, a.warppResolver(r.Context(), userID))
	diags = append(diags, a.checkSubflowCycles(r.Context(), userID, req.Document)...)
	if warpp.HasErrors(diags) {
		writeWarppJSON(w, http.StatusBadRequest, warppValidateResponse{Valid: false, Diagnostics: diags})
		return
	}
	saved, created, err := a.warppState().upsertWorkflow(r.Context(), userID, req.Document, req.Canvas)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if userID == systemUserID {
		a.syncPublishedWorkflowTools(r.Context())
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeWarppJSON(w, status, warppGetResponse{Document: saved.Document, Canvas: saved.Canvas})
}

func (a *app) deleteWarppWorkflow(w http.ResponseWriter, r *http.Request, userID int64, workflowID string) {
	deleted, err := a.warppState().deleteWorkflow(r.Context(), userID, workflowID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	if userID == systemUserID {
		a.syncPublishedWorkflowTools(r.Context())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) warppValidateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireWarppUser(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()
		var req warppValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		diags := warpp.Validate(req.Document, a.warppResolver(r.Context(), userID))
		diags = append(diags, a.checkSubflowCycles(r.Context(), userID, req.Document)...)
		writeWarppJSON(w, http.StatusOK, warppValidateResponse{Valid: !warpp.HasErrors(diags), Diagnostics: diags})
	}
}

func (a *app) warppRunsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireWarppUser(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()
		var req warppRunHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.WorkflowID) == "" {
			http.Error(w, "workflow_id required", http.StatusBadRequest)
			return
		}
		doc, _, found, err := a.warppState().getWorkflow(r.Context(), userID, req.WorkflowID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		diags := warpp.Validate(doc, a.warppResolver(r.Context(), userID))
		if warpp.HasErrors(diags) {
			writeWarppJSON(w, http.StatusUnprocessableEntity, warppValidateResponse{Valid: false, Diagnostics: diags})
			return
		}
		// A per-run project_id overrides the workflow's saved default.
		if pid := strings.TrimSpace(req.ProjectID); pid != "" {
			doc.ProjectID = pid
		}
		ctx := context.WithoutCancel(r.Context())
		if a.durableClient != nil {
			a.spawnDurableWarppRun(w, ctx, userID, doc, req.Input)
			return
		}
		runID := a.warppState().createRun(userID, doc.ID, req.Input)
		seconds := a.cfg.WorkflowTimeoutSeconds
		if seconds <= 0 {
			seconds = a.cfg.AgentRunTimeoutSeconds
		}
		go func() {
			runCtx, cancel, _ := withMaybeTimeout(ctx, seconds)
			defer cancel()
			a.executeWarppRun(runCtx, userID, runID, doc, req.Input)
		}()
		writeWarppJSON(w, http.StatusAccepted, warppRunHTTPResponse{RunID: runID, Status: warpp.StatusRunning})
	}
}

func (a *app) spawnDurableWarppRun(w http.ResponseWriter, ctx context.Context, userID int64, doc warpp.Document, input map[string]any) {
	spawn, err := a.durableClient.Spawn(ctx, durable.SpawnRequest{
		Queue:  warppDurableQueue,
		Name:   warppDurableRunTaskName,
		UserID: userID,
		Params: map[string]any{
			"workflow_id": doc.ID,
			"input":       cloneMap(input),
			"project_id":  strings.TrimSpace(doc.ProjectID),
		},
		RetryPolicy: durable.RetryPolicy{MaxAttempts: 3, Backoff: "exponential", BaseDelaySeconds: 1, MaxDelaySeconds: 30},
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	a.warppState().createRunWithID(userID, doc.ID, spawn.TaskID, input)
	writeWarppJSON(w, http.StatusAccepted, warppRunHTTPResponse{RunID: spawn.TaskID, Status: warpp.StatusRunning})
}

func (a *app) warppRunEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireWarppUser(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		runPath := strings.TrimPrefix(r.URL.Path, "/api/warpp/runs/")
		runPath = strings.Trim(strings.TrimSpace(runPath), "/")
		if runPath == "" || !strings.HasSuffix(runPath, "events") {
			http.NotFound(w, r)
			return
		}
		runID := strings.Trim(strings.TrimSuffix(runPath, "/events"), "/")
		if runID == "" || strings.Contains(runID, "/") {
			http.NotFound(w, r)
			return
		}

		if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
			a.streamWarppEvents(w, r, userID, runID)
			return
		}
		events, status, ok := a.warppState().getRunEvents(userID, runID)
		if !ok {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		writeWarppJSON(w, http.StatusOK, map[string]any{"run_id": runID, "status": status, "events": events})
	}
}

func (a *app) streamWarppEvents(w http.ResponseWriter, r *http.Request, userID int64, runID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	snapshot, ch, done, ok := a.warppState().subscribeRun(userID, runID)
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	for _, ev := range snapshot {
		writeWarppSSE(w, fl, ev)
	}
	if done {
		return
	}
	defer a.warppState().unsubscribeRun(runID, ch)
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			writeWarppSSE(w, fl, ev)
			if ev.Type == warpp.EventRunCompleted || ev.Type == warpp.EventRunFailed || ev.Type == warpp.EventRunCancelled {
				return
			}
		}
	}
}

func (a *app) warppCatalogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireWarppUser(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		manifests := warpp.BuiltinManifests()
		manifests = append(manifests, toolnode.Manifests(toolnode.Builtin())...)
		manifests = append(manifests, toolnode.DynamicManifests(a.warppCatalogRegistry(), toolnode.CuratedToolNames())...)
		manifests = append(manifests, a.publishedWorkflowManifests(r.Context(), userID)...)
		workflows, _ := a.warppState().listWorkflowSummaries(r.Context(), userID)
		writeWarppJSON(w, http.StatusOK, warppCatalogResponse{
			Manifests: manifests,
			Coercions: [][2]string{{"number", "text"}, {"boolean", "text"}},
			Workflows: workflows,
		})
	}
}

func writeWarppJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeWarppSSE(w http.ResponseWriter, fl http.Flusher, event warpp.Event) {
	b, _ := json.Marshal(event)
	_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
	fl.Flush()
}
