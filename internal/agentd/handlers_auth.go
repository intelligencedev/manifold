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
		if !a.requireUsersAPI(w, r) {
			return
		}
		setUsersCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.Method {
		case http.MethodGet:
			a.listUsers(w, r)
		case http.MethodPost:
			a.createUser(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

type authUserInput struct {
	Email, Name, Picture, Provider, Subject string
	Roles                                   []string
}

type authUserOutput struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Picture   string    `json:"picture"`
	Provider  string    `json:"provider"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Roles     []string  `json:"roles"`
}

func (a *app) requireUsersAPI(w http.ResponseWriter, r *http.Request) bool {
	if !a.cfg.Auth.Enabled || a.authStore == nil {
		http.NotFound(w, r)
		return false
	}
	if _, ok := auth.CurrentUser(r.Context()); !ok {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func setUsersCORS(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
}

func (a *app) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	okRole, _ := a.authStore.HasRole(r.Context(), u.ID, "admin")
	if !okRole {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (a *app) listUsers(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	users, err := a.authStore.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	out := make([]authUserOutput, 0, len(users))
	for _, u := range users {
		roles, _ := a.authStore.RolesForUser(r.Context(), u.ID)
		out = append(out, authUserResponse(u, roles))
	}
	writeAuthJSON(w, out)
}

func (a *app) createUser(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	in, ok := decodeAuthUserInput(w, r)
	if !ok {
		return
	}
	usr, err := a.authStore.UpsertUser(r.Context(), &auth.User{Email: in.Email, Name: in.Name, Picture: in.Picture, Provider: in.Provider, Subject: in.Subject})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = a.authStore.SetUserRoles(r.Context(), usr.ID, in.Roles)
	roles, _ := a.authStore.RolesForUser(r.Context(), usr.ID)
	writeAuthJSON(w, authUserResponse(*usr, roles))
}

func decodeAuthUserInput(w http.ResponseWriter, r *http.Request) (authUserInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	var in authUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return authUserInput{}, false
	}
	return in, true
}

func authUserResponse(u auth.User, roles []string) authUserOutput {
	return authUserOutput{
		ID: u.ID, Email: u.Email, Name: u.Name, Picture: u.Picture, Provider: u.Provider, Subject: u.Subject,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, Roles: roles,
	}
}

func writeAuthJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (a *app) userDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.requireUsersAPI(w, r) {
			return
		}
		id, ok := userIDFromPath(w, r)
		if !ok {
			return
		}

		switch r.Method {
		case http.MethodGet:
			a.getUserDetail(w, r, id)
		case http.MethodPut:
			a.updateUserDetail(w, r, id)
		case http.MethodDelete:
			a.deleteUserDetail(w, r, id)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func userIDFromPath(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/users/"))
	if idStr == "" {
		http.NotFound(w, r)
		return 0, false
	}
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func (a *app) getUserDetail(w http.ResponseWriter, r *http.Request, id int64) {
	if !a.requireAdmin(w, r) {
		return
	}
	u, err := a.authStore.GetUserByID(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	roles, _ := a.authStore.RolesForUser(r.Context(), u.ID)
	writeAuthJSON(w, authUserResponse(*u, roles))
}

func (a *app) updateUserDetail(w http.ResponseWriter, r *http.Request, id int64) {
	if !a.requireAdmin(w, r) {
		return
	}
	in, ok := decodeAuthUserInput(w, r)
	if !ok {
		return
	}
	if err := a.authStore.UpdateUser(r.Context(), &auth.User{ID: id, Email: in.Email, Name: in.Name, Picture: in.Picture, Provider: in.Provider, Subject: in.Subject}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = a.authStore.SetUserRoles(r.Context(), id, in.Roles)
	roles, _ := a.authStore.RolesForUser(r.Context(), id)
	writeAuthJSON(w, authUserOutput{ID: id, Email: in.Email, Name: in.Name, Picture: in.Picture, Provider: in.Provider, Subject: in.Subject, Roles: roles})
}

func (a *app) deleteUserDetail(w http.ResponseWriter, r *http.Request, id int64) {
	if !a.requireAdmin(w, r) {
		return
	}
	if err := a.authStore.DeleteUser(r.Context(), id); err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
