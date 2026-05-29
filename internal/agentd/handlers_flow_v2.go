package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"manifold/internal/durable"
	"manifold/internal/flow"
	persist "manifold/internal/persistence"
)

func (a *app) flowV2WorkflowsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireFlowV2User(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		workflows, err := a.flowV2State().listWorkflowSummaries(r.Context(), userID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		resp := flow.ListWorkflowsResponse{Workflows: workflows}
		writeFlowV2JSON(w, http.StatusOK, resp)
	}
}

func (a *app) flowV2WorkflowDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireFlowV2User(w, r)
		if !ok {
			return
		}
		workflowID, ok := flowV2WorkflowID(w, r)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			a.getFlowV2Workflow(w, r, userID, workflowID)
		case http.MethodPut:
			a.putFlowV2Workflow(w, r, userID, workflowID)
		case http.MethodDelete:
			a.deleteFlowV2Workflow(w, r, userID, workflowID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func flowV2WorkflowID(w http.ResponseWriter, r *http.Request) (string, bool) {
	workflowPath := strings.TrimPrefix(r.URL.EscapedPath(), "/api/flows/v2/workflows/")
	workflowPath = strings.Trim(strings.TrimSpace(workflowPath), "/")
	if workflowPath == "" {
		http.NotFound(w, r)
		return "", false
	}
	workflowID, err := url.PathUnescape(workflowPath)
	if err != nil {
		http.Error(w, "bad workflow id", http.StatusBadRequest)
		return "", false
	}
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		http.NotFound(w, r)
		return "", false
	}
	return workflowID, true
}

func (a *app) getFlowV2Workflow(w http.ResponseWriter, r *http.Request, userID int64, workflowID string) {
	wf, canvas, found, err := a.flowV2State().getWorkflow(r.Context(), userID, workflowID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	writeFlowV2JSON(w, http.StatusOK, flow.GetWorkflowResponse{Workflow: wf, Canvas: canvas})
}

func (a *app) putFlowV2Workflow(w http.ResponseWriter, r *http.Request, userID int64, workflowID string) {
	req, ok := decodeFlowV2PutWorkflow(w, r, workflowID)
	if !ok {
		return
	}
	diags := flow.ValidateWorkflow(req.Workflow)
	if hasFlowV2Errors(diags) {
		writeFlowV2JSON(w, http.StatusBadRequest, flow.ValidateResponse{Valid: false, Diagnostics: diags})
		return
	}
	saved, created, err := a.flowV2State().upsertWorkflow(r.Context(), userID, req.Workflow, req.Canvas)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if userID == systemUserID {
		a.syncWarppTools(r.Context())
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeFlowV2JSON(w, status, flow.GetWorkflowResponse{Workflow: saved.Workflow, Canvas: saved.Canvas})
}

func decodeFlowV2PutWorkflow(w http.ResponseWriter, r *http.Request, workflowID string) (flow.PutWorkflowRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	var req flow.PutWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return flow.PutWorkflowRequest{}, false
	}
	if strings.TrimSpace(req.Workflow.ID) == "" {
		req.Workflow.ID = workflowID
	}
	if req.Workflow.ID != workflowID {
		http.Error(w, "workflow id mismatch", http.StatusBadRequest)
		return flow.PutWorkflowRequest{}, false
	}
	return req, true
}

func (a *app) deleteFlowV2Workflow(w http.ResponseWriter, r *http.Request, userID int64, workflowID string) {
	deleted, err := a.flowV2State().deleteWorkflow(r.Context(), userID, workflowID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	if userID == systemUserID {
		a.syncWarppTools(r.Context())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) flowV2ValidateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.requireFlowV2User(w, r); !ok {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()

		var req flow.ValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		plan, diags := flow.CompileWorkflow(req.Workflow)
		resp := flow.ValidateResponse{
			Valid:       !hasFlowV2Errors(diags),
			Diagnostics: diags,
			Plan:        plan,
		}
		writeFlowV2JSON(w, http.StatusOK, resp)
	}
}

func (a *app) flowV2RunHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireFlowV2User(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, ok := decodeFlowV2RunRequest(w, r)
		if !ok {
			return
		}
		wf, plan, ok := a.flowV2RunWorkflow(w, r, userID, req.WorkflowID)
		if !ok {
			return
		}
		ctx, ok := a.flowV2RunContext(w, r, userID, req.ProjectID)
		if !ok {
			return
		}
		if a.durableClient != nil {
			a.spawnDurableFlowV2Run(w, ctx, userID, wf, req)
			return
		}
		a.startLocalFlowV2Run(w, ctx, userID, wf, plan, req.Input)
	}
}

func decodeFlowV2RunRequest(w http.ResponseWriter, r *http.Request) (flow.RunRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	defer r.Body.Close()
	var req flow.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return flow.RunRequest{}, false
	}
	if strings.TrimSpace(req.WorkflowID) == "" {
		http.Error(w, "workflow_id required", http.StatusBadRequest)
		return flow.RunRequest{}, false
	}
	return req, true
}

func (a *app) flowV2RunWorkflow(w http.ResponseWriter, r *http.Request, userID int64, workflowID string) (flow.Workflow, *flow.Plan, bool) {
	wf, _, found, err := a.flowV2State().getWorkflow(r.Context(), userID, workflowID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return flow.Workflow{}, nil, false
	}
	if !found {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return flow.Workflow{}, nil, false
	}
	plan, diags := flow.CompileWorkflow(wf)
	if hasFlowV2Errors(diags) || plan == nil {
		writeFlowV2JSON(w, http.StatusUnprocessableEntity, flow.ValidateResponse{Valid: false, Diagnostics: diags})
		return flow.Workflow{}, nil, false
	}
	return wf, plan, true
}

func (a *app) flowV2RunContext(w http.ResponseWriter, r *http.Request, userID int64, projectID string) (context.Context, bool) {
	ctx := context.WithoutCancel(r.Context())
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ctx, true
	}
	ctx, err := workflowToolContext(ctx, a.cfg, userID, projectID)
	if err != nil {
		http.Error(w, "invalid project_id", http.StatusBadRequest)
		return nil, false
	}
	return ctx, true
}

func (a *app) spawnDurableFlowV2Run(w http.ResponseWriter, ctx context.Context, userID int64, wf flow.Workflow, req flow.RunRequest) {
	spawn, err := a.durableClient.Spawn(ctx, durable.SpawnRequest{
		Queue:  durableFlowQueue,
		Name:   durableFlowRunTaskName,
		UserID: userID,
		Params: map[string]any{
			"workflow_id": wf.ID,
			"input":       cloneMap(req.Input),
			"project_id":  strings.TrimSpace(req.ProjectID),
		},
		RetryPolicy: durable.RetryPolicy{MaxAttempts: 3, Backoff: "exponential", BaseDelaySeconds: 1, MaxDelaySeconds: 30},
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	a.flowV2State().createRunWithID(userID, wf.ID, spawn.TaskID, req.Input)
	writeFlowV2JSON(w, http.StatusAccepted, flow.RunResponse{RunID: spawn.TaskID, Status: "running"})
}

func (a *app) startLocalFlowV2Run(w http.ResponseWriter, ctx context.Context, userID int64, wf flow.Workflow, plan *flow.Plan, input map[string]any) {
	runID := a.flowV2State().createRun(userID, wf.ID, input)
	seconds := a.cfg.WorkflowTimeoutSeconds
	if seconds <= 0 {
		seconds = a.cfg.AgentRunTimeoutSeconds
	}
	go func() {
		runCtx, cancel, timeout := withMaybeTimeout(ctx, seconds)
		_ = timeout
		defer cancel()
		a.executeFlowV2Run(runCtx, userID, runID, wf, plan, input)
	}()
	writeFlowV2JSON(w, http.StatusAccepted, flow.RunResponse{RunID: runID, Status: "running"})
}

func (a *app) flowV2RunEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.requireFlowV2User(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		runPath := strings.TrimPrefix(r.URL.Path, "/api/flows/v2/runs/")
		runPath = strings.Trim(strings.TrimSpace(runPath), "/")
		if runPath == "" || !strings.HasSuffix(runPath, "events") {
			http.NotFound(w, r)
			return
		}
		runID := strings.TrimSuffix(runPath, "/events")
		runID = strings.Trim(runID, "/")
		if runID == "" || strings.Contains(runID, "/") {
			http.NotFound(w, r)
			return
		}

		if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}
			snapshot, ch, done, ok := a.flowV2State().subscribeRun(userID, runID)
			if !ok {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			for _, ev := range snapshot {
				writeFlowV2SSE(w, fl, ev)
			}
			if done {
				return
			}
			defer a.flowV2State().unsubscribeRun(runID, ch)
			for {
				select {
				case <-r.Context().Done():
					return
				case ev := <-ch:
					writeFlowV2SSE(w, fl, ev)
					if ev.Type == flow.RunEventTypeRunCompleted || ev.Type == flow.RunEventTypeRunFailed || ev.Type == flow.RunEventTypeRunCancelled {
						return
					}
				}
			}
		}

		events, status, ok := a.flowV2State().getRunEvents(userID, runID)
		if !ok {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		writeFlowV2JSON(w, http.StatusOK, map[string]any{
			"run_id": runID,
			"status": status,
			"events": events,
		})
	}
}

func (a *app) requireFlowV2User(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, err := a.requireUserID(r)
	if err != nil {
		if a.cfg.Auth.Enabled {
			w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	return userID, true
}

func hasFlowV2Errors(diags []flow.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == flow.DiagnosticSeverityError {
			return true
		}
	}
	return false
}

func writeFlowV2JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeFlowV2SSE(w http.ResponseWriter, fl http.Flusher, event flow.RunEvent) {
	b, _ := json.Marshal(event)
	_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
	fl.Flush()
}

func (a *app) flowV2State() *flowV2Runtime {
	if a.flowV2 == nil {
		var store persist.FlowV2WorkflowStore
		if a.mgr != nil {
			store = a.mgr.FlowV2
		}
		a.flowV2 = newFlowV2Runtime(store)
	}
	return a.flowV2
}
