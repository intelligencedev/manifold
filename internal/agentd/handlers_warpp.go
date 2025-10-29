package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"path/filepath"

	"github.com/rs/zerolog/log"

	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/warpp"
)

func (a *app) warppWorkflowsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil {
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
		w.Header().Set("Content-Type", "application/json")
		reg, err := a.warppRegistryForUser(r.Context(), userID)
		if err != nil {
			log.Error().Err(err).Msg("warpp_registry_for_user")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		list := []warpp.Workflow{}
		if reg != nil {
			list = append(list, reg.All()...)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Intent < list[j].Intent })
		json.NewEncoder(w).Encode(list)
	}
}

func (a *app) warppWorkflowDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		intent := strings.TrimPrefix(r.URL.Path, "/api/warpp/workflows/")
		intent = strings.TrimSpace(intent)
		if intent == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			reg, err := a.warppRegistryForUser(r.Context(), userID)
			if err != nil {
				log.Error().Err(err).Msg("warpp_registry_for_user")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			wf, err := reg.Get(intent)
			if err != nil {
				http.Error(w, "workflow not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(wf)
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var wf warpp.Workflow
			if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if wf.Intent == "" {
				wf.Intent = intent
			}
			if wf.Intent != intent {
				http.Error(w, "intent mismatch", http.StatusBadRequest)
				return
			}
			if len(wf.Steps) == 0 {
				http.Error(w, "workflow requires steps", http.StatusBadRequest)
				return
			}
			seen := make(map[string]struct{}, len(wf.Steps))
			for _, step := range wf.Steps {
				if step.ID == "" {
					http.Error(w, "step id required", http.StatusBadRequest)
					return
				}
				if _, ok := seen[step.ID]; ok {
					http.Error(w, "duplicate step id", http.StatusBadRequest)
					return
				}
				seen[step.ID] = struct{}{}
				if step.Tool != nil && step.Tool.Name == "" {
					http.Error(w, "tool name required", http.StatusBadRequest)
					return
				}
			}
			_, existed, _ := a.warppStore.Get(r.Context(), userID, intent)
			var pw persist.WarppWorkflow
			if b, err := json.Marshal(wf); err == nil {
				_ = json.Unmarshal(b, &pw)
			}
			pw.UserID = userID
			if _, err := a.warppStore.Upsert(r.Context(), userID, pw); err != nil {
				http.Error(w, "failed to save workflow", http.StatusInternalServerError)
				return
			}
			a.invalidateWarppCache(r.Context(), userID)
			status := http.StatusOK
			if !existed {
				status = http.StatusCreated
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(wf)
		case http.MethodDelete:
			if err := a.warppStore.Delete(r.Context(), userID, intent); err != nil {
				http.Error(w, "failed to delete", http.StatusInternalServerError)
				return
			}
			a.invalidateWarppCache(r.Context(), userID)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) warppRunHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()
		var req struct {
			Intent string `json:"intent"`
			Prompt string `json:"prompt"`
			ProjectID string `json:"project_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if p := strings.TrimSpace(req.ProjectID); p != "" {
			base := filepath.Join(a.cfg.Workdir, "users", fmt.Sprint(userID), "projects", p)
			r = r.WithContext(sandbox.WithBaseDir(r.Context(), base))
		}
		intent := strings.TrimSpace(req.Intent)
		if intent == "" {
			http.Error(w, "intent required", http.StatusBadRequest)
			return
		}
		reg, err := a.warppRegistryForUser(r.Context(), userID)
		if err != nil {
			log.Error().Err(err).Msg("warpp_registry_for_user")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		wf, err := reg.Get(intent)
		if err != nil {
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			prompt = "(ui) run workflow"
		}
		attrs := warpp.Attrs{"utter": prompt}
		seconds := a.cfg.WorkflowTimeoutSeconds
		if seconds <= 0 {
			seconds = a.cfg.AgentRunTimeoutSeconds
		}
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/api/warpp/run").Msg("using configured workflow timeout")
		} else {
			log.Debug().Str("endpoint", "/api/warpp/run").Msg("no timeout configured; running until completion")
		}
		runner, err := a.warppRunnerForUser(ctx, userID)
		if err != nil {
			log.Error().Err(err).Msg("warpp_runner_for_user")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		wfStar, _, attrs2, err := runner.Personalize(ctx, wf, attrs)
		if err != nil {
			log.Error().Err(err).Msg("warpp_personalize")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		allow := map[string]bool{}
		for _, s := range wfStar.Steps {
			if s.Tool != nil {
				allow[s.Tool.Name] = true
			}
		}
		result, trace, err := runner.ExecuteWithTrace(ctx, wfStar, allow, attrs2, nil)
		if err != nil {
			log.Error().Err(err).Msg("warpp_execute")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "trace": trace})
	}
}
