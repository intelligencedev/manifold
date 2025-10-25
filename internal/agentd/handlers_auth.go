package agentd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"manifold/internal/auth"
)

func (a *app) authLoginHandler() http.HandlerFunc {
	if a.authProvider == nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}
	}
	return a.authProvider.LoginHandler()
}

func (a *app) authCallbackHandler() http.HandlerFunc {
	if a.authProvider == nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}
	}
	return a.authProvider.CallbackHandler(a.cfg.Auth.CookieSecure, a.cfg.Auth.CookieDomain)
}

func (a *app) authLogoutHandler() http.HandlerFunc {
	if a.authProvider == nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}
	}
	logout := a.authProvider.LogoutHandler(a.cfg.Auth.CookieSecure, a.cfg.Auth.CookieDomain)
	return func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		logout(w, r)
	}
}

func (a *app) meHandler() http.HandlerFunc {
	if a.authProvider == nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}
	}
	return a.authProvider.MeHandler()
}

func (a *app) usersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.Auth.Enabled || a.authStore == nil {
			http.NotFound(w, r)
			return
		}
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if u, ok := auth.CurrentUser(r.Context()); ok {
				okRole, _ := a.authStore.HasRole(r.Context(), u.ID, "admin")
				if !okRole {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			users, err := a.authStore.ListUsers(r.Context())
			if err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			type userOut struct {
				ID        int64     `json:"id"`
				Email     string    `json:"email"`
				Name      string    `json:"name"`
				Picture   string    `json:"picture"`
				Provider  string    `json:"provider"`
				Subject   string    `json:"subject"`
				CreatedAt time.Time `json:"created_at"`
				UpdatedAt time.Time `json:"updated_at"`
				Roles     []string  `json:"roles"`
			}
			out := make([]userOut, 0, len(users))
			for _, u := range users {
				roles, _ := a.authStore.RolesForUser(r.Context(), u.ID)
				out = append(out, userOut{
					ID: u.ID, Email: u.Email, Name: u.Name, Picture: u.Picture, Provider: u.Provider, Subject: u.Subject,
					CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, Roles: roles,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			if u, ok := auth.CurrentUser(r.Context()); ok {
				okRole, _ := a.authStore.HasRole(r.Context(), u.ID, "admin")
				if !okRole {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var in struct {
				Email, Name, Picture, Provider, Subject string
				Roles                                   []string
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			u := &auth.User{Email: in.Email, Name: in.Name, Picture: in.Picture, Provider: in.Provider, Subject: in.Subject}
			usr, err := a.authStore.UpsertUser(r.Context(), u)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = a.authStore.SetUserRoles(r.Context(), usr.ID, in.Roles)
			roles, _ := a.authStore.RolesForUser(r.Context(), usr.ID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id": usr.ID, "email": usr.Email, "name": usr.Name, "picture": usr.Picture,
				"provider": usr.Provider, "subject": usr.Subject, "created_at": usr.CreatedAt,
				"updated_at": usr.UpdatedAt, "roles": roles,
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) userDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.Auth.Enabled || a.authStore == nil {
			http.NotFound(w, r)
			return
		}
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/users/")
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			http.NotFound(w, r)
			return
		}
		var id int64
		if _, err := fmt.Sscan(idStr, &id); err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
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
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			u, err := a.authStore.GetUserByID(r.Context(), id)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			roles, _ := a.authStore.RolesForUser(r.Context(), u.ID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id": u.ID, "email": u.Email, "name": u.Name, "picture": u.Picture,
				"provider": u.Provider, "subject": u.Subject, "created_at": u.CreatedAt,
				"updated_at": u.UpdatedAt, "roles": roles,
			})
		case http.MethodPut:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var in struct {
				Email, Name, Picture, Provider, Subject string
				Roles                                   []string
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			u := &auth.User{ID: id, Email: in.Email, Name: in.Name, Picture: in.Picture, Provider: in.Provider, Subject: in.Subject}
			if err := a.authStore.UpdateUser(r.Context(), u); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = a.authStore.SetUserRoles(r.Context(), id, in.Roles)
			roles, _ := a.authStore.RolesForUser(r.Context(), id)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id": id, "email": in.Email, "name": in.Name, "picture": in.Picture,
				"provider": in.Provider, "subject": in.Subject, "roles": roles,
			})
		case http.MethodDelete:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if err := a.authStore.DeleteUser(r.Context(), id); err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
