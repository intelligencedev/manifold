package mcpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence"
)

// ServerHandlerDeps contains persistence and manager callbacks for the MCP
// server CRUD endpoints. MCP client lifecycle remains owned by the caller.
type ServerHandlerDeps struct {
	Store             persistence.MCPStore
	ConfigServers     func() []config.MCPServerConfig
	RequireUserID     func(*http.Request) (int64, error)
	AuthEnabled       bool
	RefreshAndConvert func(context.Context, persistence.MCPServer) (config.MCPServerConfig, bool, error)
	Register          func(context.Context, config.MCPServerConfig) error
	Remove            func(string)
	RefreshTools      func()
}

// ServerResponse is the redacted API representation of a configured MCP
// server. Secrets are intentionally omitted from this response.
type ServerResponse struct {
	ID               int64             `json:"id"`
	Name             string            `json:"name"`
	Command          string            `json:"command"`
	Args             []string          `json:"args"`
	Env              map[string]string `json:"env"`
	URL              string            `json:"url"`
	Headers          map[string]string `json:"headers"`
	Origin           string            `json:"origin"`
	ProtocolVersion  string            `json:"protocolVersion"`
	KeepAliveSeconds int               `json:"keepAliveSeconds"`
	Disabled         bool              `json:"disabled"`
	OAuthClientID    string            `json:"oauthClientId,omitempty"`
	Source           string            `json:"source"`
	Status           string            `json:"status"`
	HasToken         bool              `json:"hasToken"`
}

// ServersHandler returns the collection handler for MCP server records.
func ServersHandler(deps ServerHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireUser(w, r, deps)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			listServers(w, r, deps, userID)
		case http.MethodPost:
			createServer(w, r, deps, userID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ServerDetailHandler returns the update/delete handler for one MCP server.
func ServerDetailHandler(deps ServerHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := requireUser(w, r, deps)
		if !ok {
			return
		}
		name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/mcp/servers/"))
		if name == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			updateServer(w, r, deps, userID, name)
		case http.MethodDelete:
			deleteServer(w, r, deps, userID, name)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func requireUser(w http.ResponseWriter, r *http.Request, deps ServerHandlerDeps) (int64, bool) {
	if deps.RequireUserID == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	userID, err := deps.RequireUserID(r)
	if err == nil {
		return userID, true
	}
	if deps.AuthEnabled {
		w.Header().Set("WWW-Authenticate", `Bearer realm="sio"`)
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return 0, false
}

func listServers(w http.ResponseWriter, r *http.Request, deps ServerHandlerDeps, userID int64) {
	if deps.Store == nil {
		http.Error(w, "MCP store unavailable", http.StatusServiceUnavailable)
		return
	}
	dbServers, err := deps.Store.List(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	configServers := []config.MCPServerConfig(nil)
	if deps.ConfigServers != nil {
		configServers = deps.ConfigServers()
	}
	dbNames := make(map[string]bool, len(dbServers))
	for _, server := range dbServers {
		dbNames[server.Name] = true
	}
	out := make([]ServerResponse, 0, len(configServers)+len(dbServers))
	for _, server := range configServers {
		if dbNames[server.Name] {
			continue
		}
		out = append(out, ServerResponse{
			Name:             server.Name,
			Command:          server.Command,
			Args:             server.Args,
			Env:              server.Env,
			URL:              server.URL,
			Headers:          server.Headers,
			Origin:           server.Origin,
			ProtocolVersion:  server.ProtocolVersion,
			KeepAliveSeconds: server.KeepAliveSeconds,
			Source:           "config",
			Status:           "connected",
			HasToken:         server.BearerToken != "",
		})
	}
	for _, server := range dbServers {
		status := "connected"
		if server.Disabled {
			status = "disabled"
		} else if server.URL != "" && server.OAuthAccessToken != "" && !server.OAuthExpiresAt.IsZero() && server.OAuthExpiresAt.Before(time.Now()) && server.OAuthRefreshToken == "" {
			status = "needs_auth"
		}
		out = append(out, ServerResponse{
			ID:               server.ID,
			Name:             server.Name,
			Command:          server.Command,
			Args:             server.Args,
			Env:              server.Env,
			URL:              server.URL,
			Headers:          server.Headers,
			Origin:           server.Origin,
			ProtocolVersion:  server.ProtocolVersion,
			KeepAliveSeconds: server.KeepAliveSeconds,
			Disabled:         server.Disabled,
			OAuthClientID:    server.OAuthClientID,
			Source:           "db",
			Status:           status,
			HasToken:         server.OAuthAccessToken != "" || server.BearerToken != "",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func createServer(w http.ResponseWriter, r *http.Request, deps ServerHandlerDeps, userID int64) {
	if deps.Store == nil {
		http.Error(w, "MCP store unavailable", http.StatusServiceUnavailable)
		return
	}
	var server persistence.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	server.Name = strings.TrimSpace(server.Name)
	if server.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	saved, err := deps.Store.Upsert(r.Context(), userID, server)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registerServer(r, deps, saved, false)
	writeJSON(w, http.StatusCreated, saved)
}

func updateServer(w http.ResponseWriter, r *http.Request, deps ServerHandlerDeps, userID int64, name string) {
	if deps.Store == nil {
		http.Error(w, "MCP store unavailable", http.StatusServiceUnavailable)
		return
	}
	var server persistence.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	server.Name = name
	saved, err := deps.Store.Upsert(r.Context(), userID, server)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	registerServer(r, deps, saved, true)
	writeJSON(w, http.StatusOK, saved)
}

func deleteServer(w http.ResponseWriter, r *http.Request, deps ServerHandlerDeps, userID int64, name string) {
	if deps.Store == nil {
		http.Error(w, "MCP store unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := deps.Store.Delete(r.Context(), userID, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if deps.Remove != nil {
		deps.Remove(name)
	}
	if deps.RefreshTools != nil {
		deps.RefreshTools()
	}
	w.WriteHeader(http.StatusNoContent)
}

func registerServer(r *http.Request, deps ServerHandlerDeps, server persistence.MCPServer, update bool) {
	if server.URL != "" && server.OAuthAccessToken == "" && server.BearerToken == "" {
		verb := "connect"
		if update {
			verb = "reconnect"
		}
		fmt.Printf("MCP server %s: remote server needs OAuth, skipping %s\n", server.Name, verb)
		return
	}
	if deps.RefreshAndConvert == nil || deps.Register == nil {
		return
	}
	cfg, needsAuth, err := deps.RefreshAndConvert(r.Context(), server)
	if err != nil {
		fmt.Printf("MCP server %s: token refresh warning: %v\n", server.Name, err)
	}
	if needsAuth {
		fmt.Printf("MCP server %s needs re-authentication (token expired)\n", server.Name)
		return
	}
	if err := deps.Register(r.Context(), cfg); err != nil {
		verb := "connect"
		if update {
			verb = "reconnect updated"
		}
		fmt.Printf("failed to %s MCP server: %v\n", verb, err)
		return
	}
	if deps.RefreshTools != nil {
		deps.RefreshTools()
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
