package agentd

import (
	"encoding/json"
	"net/http"
	"strings"

	"manifold/internal/persistence"
	"manifold/internal/specialists"
)

func (a *app) groupsHandler() http.HandlerFunc {
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
			list, err := a.groupStore.List(r.Context(), userID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(list)
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var g persistence.SpecialistGroup
			if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			g.Name = strings.TrimSpace(g.Name)
			if g.Name == "" {
				http.Error(w, "name required", http.StatusBadRequest)
				return
			}
			g.UserID = userID
			g.Orchestrator = a.normalizeGroupOrchestrator(g.Name, g.Orchestrator)
			saved, err := a.groupStore.Upsert(r.Context(), userID, g)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(saved)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) groupDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/groups/")
		if strings.Contains(path, "/members/") {
			parts := strings.SplitN(path, "/members/", 2)
			if len(parts) != 2 {
				http.NotFound(w, r)
				return
			}
			groupName := strings.TrimSpace(parts[0])
			specialistName := strings.TrimSpace(parts[1])
			if groupName == "" || specialistName == "" {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodPut:
				if err := a.groupStore.AddMember(r.Context(), userID, groupName, specialistName); err != nil {
					if err == persistence.ErrNotFound {
						http.NotFound(w, r)
						return
					}
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			case http.MethodDelete:
				if err := a.groupStore.RemoveMember(r.Context(), userID, groupName, specialistName); err != nil {
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		name := strings.TrimSpace(path)
		if name == "" {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			g, ok, err := a.groupStore.GetByName(r.Context(), userID, name)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(g)
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var g persistence.SpecialistGroup
			if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			g.Name = name
			g.UserID = userID
			g.Orchestrator = a.normalizeGroupOrchestrator(name, g.Orchestrator)
			saved, err := a.groupStore.Upsert(r.Context(), userID, g)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(saved)
		case http.MethodDelete:
			if err := a.groupStore.Delete(r.Context(), userID, name); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) normalizeGroupOrchestrator(groupName string, sp persistence.Specialist) persistence.Specialist {
	name := strings.TrimSpace(groupName)
	orchestratorName := name + "-orchestrator"
	if strings.TrimSpace(sp.Provider) == "" {
		sp.Provider = a.cfg.LLMClient.Provider
	}
	if strings.TrimSpace(sp.Provider) == "" {
		sp.Provider = "openai"
	}
	model, baseURL, apiKey, headers, params := a.providerDefaults(sp.Provider)
	if strings.TrimSpace(sp.Model) == "" {
		sp.Model = model
	}
	if strings.TrimSpace(sp.BaseURL) == "" {
		sp.BaseURL = baseURL
	}
	if strings.TrimSpace(sp.APIKey) == "" {
		sp.APIKey = apiKey
	}
	if sp.ExtraHeaders == nil {
		sp.ExtraHeaders = headers
	}
	if sp.ExtraParams == nil {
		sp.ExtraParams = params
	}
	if strings.TrimSpace(sp.System) == "" {
		sp.System = a.cfg.SystemPrompt
		if strings.TrimSpace(sp.System) == "" {
			sp.System = specialists.DefaultOrchestratorPrompt
		}
	}
	sp.Name = orchestratorName
	if strings.TrimSpace(sp.Description) == "" {
		sp.Description = "Group orchestrator for " + groupName
	}
	sp.EnableTools = sp.EnableTools || a.cfg.EnableTools
	sp.Paused = false
	return sp
}
