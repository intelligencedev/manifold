package agentd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/codeqa"
	"manifold/internal/sandbox"
	"manifold/internal/workspaces"
)

const codeQARunTimeout = 30 * time.Minute

func (a *app) codeQARunsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setChatCORSHeaders(w, r, "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
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
		if a.codeQAService == nil {
			http.Error(w, "codeqa unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.Method {
		case http.MethodGet:
			limit := 50
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
					limit = parsed
				}
			}
			runs, err := a.codeQAService.ListRuns(r.Context(), userID, limit)
			if err != nil {
				log.Error().Err(err).Int64("user_id", userID).Msg("codeqa_list_runs_failed")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			writeWarppJSON(w, http.StatusOK, map[string]any{"runs": runs})
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var req codeqa.RunRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			repoPath, err := a.resolveCodeQARepositoryPath(r.Context(), userID, &req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.RepositoryPath = repoPath
			req.RunID = uuid.NewString()
			go a.executeCodeQARun(userID, req)
			writeWarppJSON(w, http.StatusAccepted, map[string]any{
				"run_id": req.RunID,
				"status": codeqa.StatusQueued,
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) codeQARunDetailHandler() http.HandlerFunc {
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
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if a.codeQAService == nil {
			http.Error(w, "codeqa unavailable", http.StatusServiceUnavailable)
			return
		}
		runPath := strings.TrimPrefix(r.URL.Path, "/api/codeqa/runs/")
		runPath = strings.Trim(strings.TrimSpace(runPath), "/")
		if runPath == "" {
			http.NotFound(w, r)
			return
		}
		if before, ok := strings.CutSuffix(runPath, "/events"); ok {
			a.serveCodeQARunEvents(w, r, userID, before)
			return
		}
		runID := strings.Trim(runPath, "/")
		if runID == "" || strings.Contains(runID, "/") {
			http.NotFound(w, r)
			return
		}
		run, ok, err := a.codeQAService.GetRun(r.Context(), userID, runID)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("codeqa_get_run_failed")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		writeWarppJSON(w, http.StatusOK, run)
	}
}

func (a *app) serveCodeQARunEvents(w http.ResponseWriter, r *http.Request, userID int64, runID string) {
	runID = strings.Trim(runID, "/")
	if runID == "" || strings.Contains(runID, "/") {
		http.NotFound(w, r)
		return
	}
	run, ok, err := a.codeQAService.GetRun(r.Context(), userID, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("codeqa_get_run_for_events_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		ch := a.codeQARuntime.subscribeRun(userID, runID)
		defer a.codeQARuntime.unsubscribeRun(userID, runID, ch)
		events, err := a.codeQAService.ListEvents(r.Context(), userID, runID)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("codeqa_list_events_failed")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		seen := make(map[int64]struct{}, len(events))
		for _, event := range events {
			seen[event.Sequence] = struct{}{}
			writeCodeQASSE(w, fl, event)
		}
		if run.Status == codeqa.StatusCompleted || run.Status == codeqa.StatusFailed {
			return
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-ch:
				if _, ok := seen[event.Sequence]; ok {
					continue
				}
				seen[event.Sequence] = struct{}{}
				writeCodeQASSE(w, fl, event)
				if event.Type == codeqa.RunEventCompleted || event.Type == codeqa.RunEventFailed {
					return
				}
			}
		}
	}
	events, err := a.codeQAService.ListEvents(r.Context(), userID, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("codeqa_list_events_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeWarppJSON(w, http.StatusOK, map[string]any{
		"run_id": runID,
		"status": run.Status,
		"events": events,
	})
}

func writeCodeQASSE(w http.ResponseWriter, fl http.Flusher, event codeqa.RunEvent) {
	b, _ := json.Marshal(event)
	_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
	fl.Flush()
}

func (a *app) executeCodeQARun(userID int64, req codeqa.RunRequest) {
	runCtx, cancel := context.WithTimeout(context.Background(), codeQARunTimeout)
	defer cancel()
	_, err := a.codeQAService.Run(runCtx, userID, req, func(event codeqa.RunEvent) {
		if a.codeQARuntime != nil {
			a.codeQARuntime.publish(userID, event)
		}
	})
	if err != nil {
		log.Error().Err(err).Str("run_id", req.RunID).Int64("user_id", userID).Msg("codeqa_run_failed")
	}
}

func (a *app) resolveCodeQARepositoryPath(ctx context.Context, userID int64, req *codeqa.RunRequest) (string, error) {
	if req == nil {
		return a.cfg.Workdir, nil
	}
	if projectID := strings.TrimSpace(req.ProjectID); projectID != "" {
		cleanProjectID, err := workspaces.ValidateProjectID(projectID)
		if err != nil {
			return "", fmt.Errorf("invalid project_id")
		}
		baseDir := filepath.Join(a.cfg.Workdir, "users", fmt.Sprint(userID), "projects", cleanProjectID)
		return baseDir, nil
	}
	baseDir := a.cfg.Workdir
	if projectID, ok := sandbox.ProjectIDFromContext(ctx); ok && strings.TrimSpace(projectID) != "" {
		baseDir = filepath.Join(a.cfg.Workdir, "users", fmt.Sprint(userID), "projects", projectID)
	}
	if strings.TrimSpace(req.RepositoryPath) == "" {
		return baseDir, nil
	}
	rel, err := sandbox.SanitizeArg(baseDir, req.RepositoryPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, rel), nil
}
