package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"manifold/internal/flow"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
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
		workflowID := strings.TrimPrefix(r.URL.Path, "/api/flows/v2/workflows/")
		workflowID = strings.Trim(strings.TrimSpace(workflowID), "/")
		if workflowID == "" || strings.Contains(workflowID, "/") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			wf, canvas, found, err := a.flowV2State().getWorkflow(r.Context(), userID, workflowID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !found {
				http.Error(w, "workflow not found", http.StatusNotFound)
				return
			}
			writeFlowV2JSON(w, http.StatusOK, flow.GetWorkflowResponse{
				Workflow: wf,
				Canvas:   canvas,
			})
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var req flow.PutWorkflowRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(req.Workflow.ID) == "" {
				req.Workflow.ID = workflowID
			}
			if req.Workflow.ID != workflowID {
				http.Error(w, "workflow id mismatch", http.StatusBadRequest)
				return
			}
			diags := flow.ValidateWorkflow(req.Workflow)
			if hasFlowV2Errors(diags) {
				writeFlowV2JSON(w, http.StatusBadRequest, flow.ValidateResponse{
					Valid:       false,
					Diagnostics: diags,
				})
				return
			}
			saved, created, err := a.flowV2State().upsertWorkflow(r.Context(), userID, req.Workflow, req.Canvas)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			status := http.StatusOK
			if created {
				status = http.StatusCreated
			}
			writeFlowV2JSON(w, status, flow.GetWorkflowResponse{
				Workflow: saved.Workflow,
				Canvas:   saved.Canvas,
			})
		case http.MethodDelete:
			deleted, err := a.flowV2State().deleteWorkflow(r.Context(), userID, workflowID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !deleted {
				http.Error(w, "workflow not found", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
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
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()

		var req flow.RunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.WorkflowID) == "" {
			http.Error(w, "workflow_id required", http.StatusBadRequest)
			return
		}
		wf, _, found, err := a.flowV2State().getWorkflow(r.Context(), userID, req.WorkflowID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		plan, diags := flow.CompileWorkflow(wf)
		if hasFlowV2Errors(diags) || plan == nil {
			writeFlowV2JSON(w, http.StatusUnprocessableEntity, flow.ValidateResponse{
				Valid:       false,
				Diagnostics: diags,
			})
			return
		}

		ctx := context.WithoutCancel(r.Context())
		if p := strings.TrimSpace(req.ProjectID); p != "" {
			cleanP := filepath.Clean(p)
			if cleanP != p || strings.HasPrefix(cleanP, "..") || strings.Contains(cleanP, string(filepath.Separator)+"..") || filepath.IsAbs(cleanP) {
				http.Error(w, "invalid project_id", http.StatusBadRequest)
				return
			}
			baseRoot := filepath.Join(a.cfg.Workdir, "users", fmt.Sprint(userID), "projects")
			base := filepath.Join(baseRoot, cleanP)
			if !strings.HasPrefix(base, baseRoot+string(filepath.Separator)) && base != baseRoot {
				http.Error(w, "invalid project_id", http.StatusBadRequest)
				return
			}
			ctx = sandbox.WithBaseDir(ctx, base)
			ctx = sandbox.WithProjectID(ctx, cleanP)
		}

		runID := a.flowV2State().createRun(userID, wf.ID, req.Input)
		seconds := a.cfg.WorkflowTimeoutSeconds
		if seconds <= 0 {
			seconds = a.cfg.AgentRunTimeoutSeconds
		}
		go func() {
			runCtx, cancel, _ := withMaybeTimeout(ctx, seconds)
			defer cancel()
			a.executeFlowV2Run(runCtx, userID, runID, wf, plan, req.Input)
		}()
		writeFlowV2JSON(w, http.StatusAccepted, flow.RunResponse{
			RunID:  runID,
			Status: "running",
		})
	}
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
