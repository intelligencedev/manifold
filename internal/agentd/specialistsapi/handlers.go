// Package specialistsapi owns specialist HTTP transport.
package specialistsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"manifold/internal/persistence"
)

type Deps struct {
	RequireUserID    func(*http.Request) (int64, error)
	AuthEnabled      func() bool
	List             func(context.Context, int64) ([]persistence.Specialist, error)
	Create           func(context.Context, int64, persistence.Specialist) (persistence.Specialist, int, error)
	Get              func(context.Context, int64, string) (persistence.Specialist, bool, error)
	Update           func(context.Context, int64, string, persistence.Specialist) (persistence.Specialist, error)
	Delete           func(context.Context, int64, string) error
	DeleteBadRequest func(error) bool
}

// DetailHandler serves specialist read, update, and deletion operations.
func DetailHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := authorize(w, r, deps)
		if !ok {
			return
		}
		name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/specialists/"))
		if name == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			specialist, found, err := deps.Get(r.Context(), userID, name)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !found {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, http.StatusOK, specialist)
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var specialist persistence.Specialist
			if err := json.NewDecoder(r.Body).Decode(&specialist); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			saved, err := deps.Update(r.Context(), userID, name, specialist)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, saved)
		case http.MethodDelete:
			if err := deps.Delete(r.Context(), userID, name); err != nil {
				if deps.DeleteBadRequest != nil && deps.DeleteBadRequest(err) {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else {
					http.Error(w, "error", http.StatusInternalServerError)
				}
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
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

// CollectionHandler serves specialist listing and creation.
func CollectionHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := authorize(w, r, deps)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			specialists, err := deps.List(r.Context(), userID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, specialists)
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var specialist persistence.Specialist
			if err := json.NewDecoder(r.Body).Decode(&specialist); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			saved, status, err := deps.Create(r.Context(), userID, specialist)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, status, saved)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
