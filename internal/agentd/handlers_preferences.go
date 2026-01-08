package agentd

import (
	"encoding/json"
	"net/http"

	"manifold/internal/auth"

	"github.com/rs/zerolog/log"
)

// userPreferencesHandler handles GET /api/me/preferences and PUT /api/me/preferences.
func (a *app) userPreferencesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CORS headers
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Require authentication
		userID := systemUserID
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"manifold\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			userID = u.ID
		}

		if a.userPrefsStore == nil {
			http.Error(w, "preferences not available", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			a.handleGetPreferences(w, r, userID)
		case http.MethodPut:
			a.handleSetPreferences(w, r, userID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) handleGetPreferences(w http.ResponseWriter, r *http.Request, userID int64) {
	prefs, err := a.userPrefsStore.Get(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Int64("userId", userID).Msg("failed to get user preferences")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(prefs)
}

func (a *app) handleSetPreferences(w http.ResponseWriter, r *http.Request, userID int64) {
	var req struct {
		ActiveProjectID string `json:"activeProjectId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := a.userPrefsStore.SetActiveProject(r.Context(), userID, req.ActiveProjectID); err != nil {
		log.Error().Err(err).Int64("userId", userID).Msg("failed to set active project")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// In enterprise mode, set up per-user MCP sessions for the new project
	if a.mcpPool != nil && a.mcpPool.RequiresPerUserMCP() && req.ActiveProjectID != "" {
		ws, err := a.workspaceManager.Checkout(r.Context(), userID, req.ActiveProjectID, "")
		if err != nil {
			log.Warn().Err(err).Int64("userId", userID).Str("projectId", req.ActiveProjectID).Msg("workspace_checkout_for_mcp_failed")
		} else if ws.BaseDir != "" {
			if err := a.mcpPool.EnsureUserSession(r.Context(), a.baseToolRegistry, userID, req.ActiveProjectID, ws.BaseDir); err != nil {
				log.Warn().Err(err).Int64("userId", userID).Str("projectId", req.ActiveProjectID).Msg("mcp_session_setup_failed")
			}
		}
	}

	// Return updated preferences
	prefs, err := a.userPrefsStore.Get(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Int64("userId", userID).Msg("failed to get updated preferences")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(prefs)
}

// setActiveProjectHandler handles POST /api/me/preferences/project.
// This is a convenience endpoint for setting just the active project.
func (a *app) setActiveProjectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CORS headers
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Require authentication
		userID := systemUserID
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"manifold\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			userID = u.ID
		}

		if a.userPrefsStore == nil {
			http.Error(w, "preferences not available", http.StatusServiceUnavailable)
			return
		}

		var req struct {
			ProjectID string `json:"projectId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := a.userPrefsStore.SetActiveProject(r.Context(), userID, req.ProjectID); err != nil {
			log.Error().Err(err).Int64("userId", userID).Str("projectId", req.ProjectID).Msg("failed to set active project")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// In enterprise mode, set up per-user MCP sessions for the new project
		if a.mcpPool != nil && a.mcpPool.RequiresPerUserMCP() && req.ProjectID != "" {
			// Checkout workspace to get the materialized path
			ws, err := a.workspaceManager.Checkout(r.Context(), userID, req.ProjectID, "")
			if err != nil {
				log.Warn().Err(err).Int64("userId", userID).Str("projectId", req.ProjectID).Msg("workspace_checkout_for_mcp_failed")
				// Non-fatal - preference is saved but MCP session not set up
			} else if ws.BaseDir != "" {
				if err := a.mcpPool.EnsureUserSession(r.Context(), a.baseToolRegistry, userID, req.ProjectID, ws.BaseDir); err != nil {
					log.Warn().Err(err).Int64("userId", userID).Str("projectId", req.ProjectID).Msg("mcp_session_setup_failed")
					// Non-fatal - agent can still work without path-dependent MCP tools
				}
			}
		}

		log.Debug().Int64("userId", userID).Str("projectId", req.ProjectID).Msg("active project updated")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "activeProjectId": req.ProjectID})
	}
}
