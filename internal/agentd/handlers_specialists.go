package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent/prompts"
	llmproviders "manifold/internal/llm/providers"
	persist "manifold/internal/persistence"
	"manifold/internal/tools"
)

func (a *app) statusHandler() http.HandlerFunc {
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		type agentStatus struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			State     string `json:"state"`
			Model     string `json:"model"`
			UpdatedAt string `json:"updatedAt"`
		}
		list, err := a.specStore.List(r.Context(), userID)
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

func (a *app) specialistDefaultsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "application/json")
		out := map[string]persist.Specialist{}
		for _, p := range []string{"openai", "anthropic", "google", "local"} {
			model, baseURL, apiKey, headers, params := a.providerDefaults(p)
			out[p] = persist.Specialist{
				Provider:     p,
				BaseURL:      baseURL,
				APIKey:       apiKey,
				Model:        model,
				ExtraHeaders: headers,
				ExtraParams:  params,
			}
		}
		json.NewEncoder(w).Encode(out)
	}
}

func (a *app) specialistsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			list, err := a.specStore.List(r.Context(), userID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			out := make([]persist.Specialist, 0, len(list)+1)
			out = append(out, a.orchestratorSpecialist(r.Context(), userID))
			out = append(out, list...)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)

		case http.MethodPost:
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
			if strings.TrimSpace(sp.Provider) == "" {
				sp.Provider = a.cfg.LLMClient.Provider
			}
			if name == "orchestrator" {
				// Allow non-system users to persist a per-user orchestrator overlay
				// without mutating the global engine/config.
				if userID == systemUserID {
					if err := a.applyOrchestratorUpdate(r.Context(), sp); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context(), userID))
					return
				}
				sp.Name = "orchestrator"
				sp.UserID = userID
				if _, err := a.specStore.Upsert(r.Context(), userID, sp); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context(), userID))
				a.invalidateSpecialistsCache(r.Context(), userID)
				return
			}
			sp.Name = name
			sp.UserID = userID
			saved, err := a.specStore.Upsert(r.Context(), userID, sp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(saved)
			a.invalidateSpecialistsCache(r.Context(), userID)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) specialistDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/specialists/")
		name = strings.TrimSpace(name)
		if name == "" {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if name == "orchestrator" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context(), userID))
				return
			}
			sp, ok, err := a.specStore.GetByName(r.Context(), userID, name)
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
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var sp persist.Specialist
			if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(sp.Provider) == "" {
				sp.Provider = a.cfg.LLMClient.Provider
			}
			if name == "orchestrator" {
				// Allow non-system users to update their per-user orchestrator overlay
				if userID == systemUserID {
					if err := a.applyOrchestratorUpdate(r.Context(), sp); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context(), userID))
					return
				}
				sp.Name = "orchestrator"
				sp.UserID = userID
				if _, err := a.specStore.Upsert(r.Context(), userID, sp); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(a.orchestratorSpecialist(r.Context(), userID))
				a.invalidateSpecialistsCache(r.Context(), userID)
				return
			}
			sp.Name = name
			sp.UserID = userID
			saved, err := a.specStore.Upsert(r.Context(), userID, sp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(saved)
			a.invalidateSpecialistsCache(r.Context(), userID)
		case http.MethodDelete:
			if name == "orchestrator" {
				http.Error(w, "cannot delete orchestrator", http.StatusBadRequest)
				return
			}
			if err := a.specStore.Delete(r.Context(), userID, name); err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			a.invalidateSpecialistsCache(r.Context(), userID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) orchestratorSpecialist(ctx context.Context, userID int64) persist.Specialist {
	defaultProvider := strings.TrimSpace(a.cfg.LLMClient.Provider)
	if defaultProvider == "" {
		defaultProvider = "openai"
	}
	baseModel, baseURL, baseKey, baseHeaders, baseParams := a.providerDefaults(defaultProvider)
	// Start from global defaults
	out := persist.Specialist{
		ID:           0,
		UserID:       userID,
		Name:         "orchestrator",
		Description:  "",
		Provider:     defaultProvider,
		BaseURL:      baseURL,
		APIKey:       baseKey,
		Model:        baseModel,
		EnableTools:  a.cfg.EnableTools,
		Paused:       false,
		AllowTools:   a.cfg.ToolAllowList,
		System:       a.cfg.SystemPrompt,
		ExtraHeaders: baseHeaders,
		ExtraParams:  baseParams,
	}
	// Apply per-user overlay if present
	if sp, ok, _ := a.specStore.GetByName(ctx, userID, "orchestrator"); ok {
		out.ID = sp.ID
		out.Description = sp.Description
		if strings.TrimSpace(sp.Provider) != "" {
			out.Provider = strings.TrimSpace(sp.Provider)
			// Refresh defaults for the saved provider
			out.Model, out.BaseURL, out.APIKey, out.ExtraHeaders, out.ExtraParams = a.providerDefaults(out.Provider)
		}
		if strings.TrimSpace(sp.BaseURL) != "" {
			out.BaseURL = sp.BaseURL
		}
		if strings.TrimSpace(sp.APIKey) != "" {
			out.APIKey = sp.APIKey
		}
		if strings.TrimSpace(sp.Model) != "" {
			out.Model = sp.Model
		}
		out.EnableTools = sp.EnableTools
		if sp.AllowTools != nil {
			out.AllowTools = append([]string(nil), sp.AllowTools...)
		}
		out.ReasoningEffort = sp.ReasoningEffort
		if strings.TrimSpace(sp.System) != "" {
			out.System = sp.System
		}
		if sp.ExtraHeaders != nil {
			out.ExtraHeaders = sp.ExtraHeaders
		}
		if sp.ExtraParams != nil {
			out.ExtraParams = sp.ExtraParams
		}
	}
	return out
}

func (a *app) providerDefaults(provider string) (model, baseURL, apiKey string, headers map[string]string, params map[string]any) {
	switch provider {
	case "anthropic":
		baseURL = strings.TrimSpace(a.cfg.LLMClient.Anthropic.BaseURL)
		apiKey = strings.TrimSpace(a.cfg.LLMClient.Anthropic.APIKey)
		model = strings.TrimSpace(a.cfg.LLMClient.Anthropic.Model)
	case "google":
		baseURL = strings.TrimSpace(a.cfg.LLMClient.Google.BaseURL)
		apiKey = strings.TrimSpace(a.cfg.LLMClient.Google.APIKey)
		model = strings.TrimSpace(a.cfg.LLMClient.Google.Model)
	default:
		baseURL = strings.TrimSpace(a.cfg.LLMClient.OpenAI.BaseURL)
		apiKey = strings.TrimSpace(a.cfg.LLMClient.OpenAI.APIKey)
		model = strings.TrimSpace(a.cfg.LLMClient.OpenAI.Model)
		headers = copyStringMap(a.cfg.LLMClient.OpenAI.ExtraHeaders)
		params = copyAnyMap(a.cfg.LLMClient.OpenAI.ExtraParams)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	if params == nil {
		params = map[string]any{}
	}
	return model, baseURL, apiKey, headers, params
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (a *app) applyOrchestratorUpdate(ctx context.Context, sp persist.Specialist) error {
	provider := strings.TrimSpace(sp.Provider)
	if provider == "" {
		provider = a.cfg.LLMClient.Provider
	}
	llmCfg := a.cfg.LLMClient
	llmCfg.Provider = provider

	switch provider {
	case "anthropic":
		if strings.TrimSpace(sp.BaseURL) != "" {
			llmCfg.Anthropic.BaseURL = strings.TrimSpace(sp.BaseURL)
		}
		if strings.TrimSpace(sp.APIKey) != "" {
			llmCfg.Anthropic.APIKey = strings.TrimSpace(sp.APIKey)
		}
		if strings.TrimSpace(sp.Model) != "" {
			llmCfg.Anthropic.Model = strings.TrimSpace(sp.Model)
		}
	case "google":
		if strings.TrimSpace(sp.BaseURL) != "" {
			llmCfg.Google.BaseURL = strings.TrimSpace(sp.BaseURL)
		}
		if strings.TrimSpace(sp.APIKey) != "" {
			llmCfg.Google.APIKey = strings.TrimSpace(sp.APIKey)
		}
		if strings.TrimSpace(sp.Model) != "" {
			llmCfg.Google.Model = strings.TrimSpace(sp.Model)
		}
	default:
		if strings.TrimSpace(sp.BaseURL) != "" {
			llmCfg.OpenAI.BaseURL = strings.TrimSpace(sp.BaseURL)
		}
		if strings.TrimSpace(sp.APIKey) != "" {
			llmCfg.OpenAI.APIKey = strings.TrimSpace(sp.APIKey)
		}
		if strings.TrimSpace(sp.Model) != "" {
			llmCfg.OpenAI.Model = strings.TrimSpace(sp.Model)
		}
		if sp.ExtraHeaders != nil {
			llmCfg.OpenAI.ExtraHeaders = sp.ExtraHeaders
		}
		if sp.ExtraParams != nil {
			llmCfg.OpenAI.ExtraParams = sp.ExtraParams
		}
	}

	a.cfg.LLMClient = llmCfg
	if provider == "openai" || provider == "local" || provider == "" {
		a.cfg.OpenAI = llmCfg.OpenAI
	}
	a.cfg.EnableTools = sp.EnableTools
	a.cfg.ToolAllowList = append([]string(nil), sp.AllowTools...)
	a.cfg.SystemPrompt = sp.System

	llm, err := llmproviders.Build(*a.cfg, a.httpClient)
	if err != nil {
		return err
	}
	a.llm = llm
	a.engine.LLM = llm
	currentModel := strings.TrimSpace(sp.Model)
	if currentModel == "" {
		switch provider {
		case "anthropic":
			currentModel = strings.TrimSpace(a.cfg.LLMClient.Anthropic.Model)
		case "google":
			currentModel = strings.TrimSpace(a.cfg.LLMClient.Google.Model)
		default:
			currentModel = strings.TrimSpace(a.cfg.LLMClient.OpenAI.Model)
		}
	}
	a.engine.Model = currentModel
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
		Name:        "orchestrator",
		Description: sp.Description,
		EnableTools: a.cfg.EnableTools,
		Paused:      false,
		AllowTools:  append([]string(nil), a.cfg.ToolAllowList...),
		System:      a.cfg.SystemPrompt,
		Provider:    provider,
	}
	switch provider {
	case "anthropic":
		toSave.BaseURL = a.cfg.LLMClient.Anthropic.BaseURL
		toSave.APIKey = a.cfg.LLMClient.Anthropic.APIKey
		toSave.Model = a.cfg.LLMClient.Anthropic.Model
	case "google":
		toSave.BaseURL = a.cfg.LLMClient.Google.BaseURL
		toSave.APIKey = a.cfg.LLMClient.Google.APIKey
		toSave.Model = a.cfg.LLMClient.Google.Model
	default:
		toSave.BaseURL = a.cfg.LLMClient.OpenAI.BaseURL
		toSave.APIKey = a.cfg.LLMClient.OpenAI.APIKey
		toSave.Model = a.cfg.LLMClient.OpenAI.Model
		toSave.ExtraHeaders = a.cfg.LLMClient.OpenAI.ExtraHeaders
		toSave.ExtraParams = a.cfg.LLMClient.OpenAI.ExtraParams
		toSave.ReasoningEffort = sp.ReasoningEffort
	}
	toSave.UserID = systemUserID
	if _, err := a.specStore.Upsert(ctx, systemUserID, toSave); err != nil {
		log.Error().Err(err).Msg("failed to persist orchestrator configuration")
		return err
	}
	if list, err := a.specStore.List(ctx, systemUserID); err == nil {
		a.specRegistry.ReplaceFromConfigs(a.cfg.LLMClient, specialistsFromStore(list), a.httpClient, a.baseToolRegistry)
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
