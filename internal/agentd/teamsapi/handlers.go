// Package teamsapi owns specialist-team HTTP transport.
package teamsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"manifold/internal/persistence"
)

type Deps struct {
	RequireUserID func(*http.Request) (int64, error)
	AuthEnabled   func() bool
	List          func(context.Context, int64) ([]persistence.SpecialistTeam, error)
	Create        func(context.Context, int64, persistence.SpecialistTeam) (persistence.SpecialistTeam, error)
	Get           func(context.Context, int64, string) (persistence.SpecialistTeam, bool, error)
	Update        func(context.Context, int64, string, persistence.SpecialistTeam) (persistence.SpecialistTeam, error)
	Delete        func(context.Context, int64, string) error
	AddMember     func(context.Context, int64, string, string) error
	RemoveMember  func(context.Context, int64, string, string) error
}

func authorize(w http.ResponseWriter, r *http.Request, deps Deps) (int64, bool) {
	if deps.RequireUserID == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	userID, err := deps.RequireUserID(r)
	if err == nil {
		return userID, true
	}
	if deps.AuthEnabled != nil && deps.AuthEnabled() {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return 0, false
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// CollectionHandler serves team listing and creation.
func CollectionHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := authorize(w, r, deps)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			teams, err := deps.List(r.Context(), userID)
			if err != nil {
				http.Error(w, "internal server error", 500)
				return
			}
			write(w, 200, teams)
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var team persistence.SpecialistTeam
			if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
				http.Error(w, "bad request", 400)
				return
			}
			saved, err := deps.Create(r.Context(), userID, team)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			write(w, 201, saved)
		default:
			http.Error(w, "method not allowed", 405)
		}
	}
}

// DetailHandler serves a team and its membership operations.
func DetailHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := authorize(w, r, deps)
		if !ok {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/teams/")
		if strings.Contains(path, "/members/") {
			parts := strings.SplitN(path, "/members/", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				http.NotFound(w, r)
				return
			}
			var err error
			switch r.Method {
			case http.MethodPut:
				err = deps.AddMember(r.Context(), userID, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			case http.MethodDelete:
				err = deps.RemoveMember(r.Context(), userID, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			default:
				http.Error(w, "method not allowed", 405)
				return
			}
			if err != nil {
				if err == persistence.ErrNotFound {
					http.NotFound(w, r)
				} else {
					http.Error(w, "internal server error", 500)
				}
				return
			}
			w.WriteHeader(204)
			return
		}
		name := strings.TrimSpace(path)
		if name == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			team, found, err := deps.Get(r.Context(), userID, name)
			if err != nil {
				http.Error(w, "internal server error", 500)
				return
			}
			if !found {
				http.NotFound(w, r)
				return
			}
			write(w, 200, team)
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var team persistence.SpecialistTeam
			if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
				http.Error(w, "bad request", 400)
				return
			}
			saved, err := deps.Update(r.Context(), userID, name, team)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			write(w, 200, saved)
		case http.MethodDelete:
			if err := deps.Delete(r.Context(), userID, name); err != nil {
				http.Error(w, "internal server error", 500)
				return
			}
			w.WriteHeader(204)
		default:
			http.Error(w, "method not allowed", 405)
		}
	}
}
