package agentd

import (
	"encoding/json"
	"net/http"
	"strings"

	"manifold/internal/persistence"
)

func (a *app) teamsHandler() http.HandlerFunc {
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
			list, err := a.listTeamsForUser(r.Context(), userID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(list)
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var g persistence.SpecialistTeam
			if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			saved, err := a.createTeamForUser(r.Context(), userID, g)
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

func (a *app) teamDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil {
			if a.cfg.Auth.Enabled {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/teams/")
		if strings.Contains(path, "/members/") {
			teamName, specialistName, ok := parseTeamMemberPath(path)
			if !ok {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodPut:
				if err := a.addSpecialistToTeamForUser(r.Context(), userID, teamName, specialistName); err != nil {
					if err == persistence.ErrNotFound {
						http.NotFound(w, r)
						return
					}
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			case http.MethodDelete:
				if err := a.removeSpecialistFromTeamForUser(r.Context(), userID, teamName, specialistName); err != nil {
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
			g, ok, err := a.getTeamForUser(r.Context(), userID, name)
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
			var g persistence.SpecialistTeam
			if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			saved, err := a.updateTeamForUser(r.Context(), userID, name, g)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(saved)
		case http.MethodDelete:
			if err := a.deleteTeamForUser(r.Context(), userID, name); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
