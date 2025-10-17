package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent/prompts"
	"manifold/internal/auth"
	openaillm "manifold/internal/llm/openai"
	persist "manifold/internal/persistence"
	"manifold/internal/tools"
)

func (a *app) statusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		type agentStatus struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			State     string `json:"state"`
			Model     string `json:"model"`
			UpdatedAt string `json:"updatedAt"`
		}
		list, err := a.specStore.List(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		out := make([]agentStatus, 0, len(list))
		for _, s := range list {
			if s.Paused {
				// Maintain previous behaviour: paused specialists are hidden
				// from the Overview cards so they read as "offline".
				continue
			}
			out = append(out, agentStatus{
				ID:        s.Name,
				Name:      s.Name,
				State:     "online",
				Model:     s.Model,
				UpdatedAt: now,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

func (a *app) specialistsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		switch r.Method {
		case http.MethodGet:
			list, err := a.specStore.List(r.Context())
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			out := make([]persist.Specialist, 0, len(list)+1)
			out = append(out, a.orchestratorSpecialist(r.Context()))
			out = append(out, list...)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)

		case http.MethodPost:
			// Create a new specialist (admin only)
			isAdmin := false
			if u, ok := auth.CurrentUser(r.Context()); ok {
				okRole, _ := a.authStore.HasRole(r.Context(), u.ID, "admin")
				if okRole {
					isAdmin = true
				}
			}
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var sp persist.Specialist
			if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			name := strings.TrimSpace(sp.Name)
			if name == "" {
				http.Error(w, "name required", http.StatusBadRequest)
				return
			}
			if name == "orchestrator" {
				if err := a.applyOrchestratorUpdate(r.Context(), sp); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context()))
				return
			}
			sp.Name = name
			saved, err := a.specStore.Upsert(r.Context(), sp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(saved)
			if list, err := a.specStore.List(r.Context()); err == nil {
				a.specRegistry.ReplaceFromConfigs(a.cfg.OpenAI, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
			}

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) specialistDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/specialists/")
		name = strings.TrimSpace(name)
		if name == "" {
			http.NotFound(w, r)
			return
		}

		isAdmin := false
		if u, ok := auth.CurrentUser(r.Context()); ok {
			okRole, _ := a.authStore.HasRole(r.Context(), u.ID, "admin")
			if okRole {
				isAdmin = true
			}
		}

		switch r.Method {
		case http.MethodGet:
			if name == "orchestrator" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context()))
				return
			}
			sp, ok, err := a.specStore.GetByName(r.Context(), name)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sp)
		case http.MethodPut:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var sp persist.Specialist
			if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if name == "orchestrator" {
				if err := a.applyOrchestratorUpdate(r.Context(), sp); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context()))
				return
			}
			sp.Name = name
			saved, err := a.specStore.Upsert(r.Context(), sp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(saved)
			if list, err := a.specStore.List(r.Context()); err == nil {
				a.specRegistry.ReplaceFromConfigs(a.cfg.OpenAI, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
			}
		case http.MethodDelete:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if name == "orchestrator" {
				http.Error(w, "cannot delete orchestrator", http.StatusBadRequest)
				return
			}
			if err := a.specStore.Delete(r.Context(), name); err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			if list, err := a.specStore.List(r.Context()); err == nil {
				a.specRegistry.ReplaceFromConfigs(a.cfg.OpenAI, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) orchestratorSpecialist(ctx context.Context) persist.Specialist {
	out := persist.Specialist{
		ID:           0,
		Name:         "orchestrator",
		Description:  "",
		BaseURL:      a.cfg.OpenAI.BaseURL,
		APIKey:       a.cfg.OpenAI.APIKey,
		Model:        a.cfg.OpenAI.Model,
		EnableTools:  a.cfg.EnableTools,
		Paused:       false,
		AllowTools:   a.cfg.ToolAllowList,
		System:       a.cfg.SystemPrompt,
		ExtraHeaders: a.cfg.OpenAI.ExtraHeaders,
		ExtraParams:  a.cfg.OpenAI.ExtraParams,
	}
	if sp, ok, _ := a.specStore.GetByName(ctx, "orchestrator"); ok {
		out.ReasoningEffort = sp.ReasoningEffort
		out.Description = sp.Description
	}
	return out
}

func (a *app) applyOrchestratorUpdate(ctx context.Context, sp persist.Specialist) error {
	a.cfg.OpenAI.BaseURL = sp.BaseURL
	a.cfg.OpenAI.APIKey = sp.APIKey
	if strings.TrimSpace(sp.Model) != "" {
		a.cfg.OpenAI.Model = sp.Model
	}
	a.cfg.EnableTools = sp.EnableTools
	a.cfg.ToolAllowList = append([]string(nil), sp.AllowTools...)
	a.cfg.SystemPrompt = sp.System
	if sp.ExtraHeaders != nil {
		a.cfg.OpenAI.ExtraHeaders = sp.ExtraHeaders
	}
	if sp.ExtraParams != nil {
		a.cfg.OpenAI.ExtraParams = sp.ExtraParams
	}

	llm := openaillm.New(a.cfg.OpenAI, a.httpClient)
	a.engine.LLM = llm
	a.engine.Model = a.cfg.OpenAI.Model
	a.engine.System = prompts.DefaultSystemPrompt(a.cfg.Workdir, a.cfg.SystemPrompt)

	if !a.cfg.EnableTools {
		a.toolRegistry = tools.NewRegistry()
	} else if len(a.cfg.ToolAllowList) > 0 {
		a.toolRegistry = tools.NewFilteredRegistry(a.baseToolRegistry, a.cfg.ToolAllowList)
	} else {
		a.toolRegistry = a.baseToolRegistry
	}

	a.engine.Tools = a.toolRegistry
	a.warppMu.Lock()
	a.warppRunner.Tools = a.toolRegistry
	a.warppMu.Unlock()

	toSave := persist.Specialist{
		Name:            "orchestrator",
		Description:     sp.Description,
		BaseURL:         a.cfg.OpenAI.BaseURL,
		APIKey:          a.cfg.OpenAI.APIKey,
		Model:           a.cfg.OpenAI.Model,
		EnableTools:     a.cfg.EnableTools,
		Paused:          false,
		AllowTools:      append([]string(nil), a.cfg.ToolAllowList...),
		ReasoningEffort: sp.ReasoningEffort,
		System:          a.cfg.SystemPrompt,
		ExtraHeaders:    a.cfg.OpenAI.ExtraHeaders,
		ExtraParams:     a.cfg.OpenAI.ExtraParams,
	}
	if _, err := a.specStore.Upsert(ctx, toSave); err != nil {
		log.Error().Err(err).Msg("failed to persist orchestrator configuration")
		return err
	}
	if list, err := a.specStore.List(ctx); err == nil {
		a.specRegistry.ReplaceFromConfigs(a.cfg.OpenAI, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
	}
	names := make([]string, 0, len(a.toolRegistry.Schemas()))
	for _, s := range a.toolRegistry.Schemas() {
		names = append(names, s.Name)
	}
	log.Info().
		Bool("enableTools", a.cfg.EnableTools).
		Strs("allowList", a.cfg.ToolAllowList).
		Strs("tools", names).
		Msg("tool_registry_contents_updated")
	return nil
}
