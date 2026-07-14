package mcpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// OAuthStartRequest is the public API payload used to begin MCP OAuth.
type OAuthStartRequest struct {
	ServerID int64
	URL      string
}

// OAuthHandlerDeps contains the narrow application callbacks required by the
// OAuth HTTP surface. Discovery, registration, and client lifecycle remain at
// the composition root.
type OAuthHandlerDeps struct {
	RequireUserID   func(*http.Request) (int64, error)
	AuthEnabled     func() bool
	SystemUserID    int64
	PrepareRedirect func(http.ResponseWriter, *http.Request, int64, OAuthStartRequest) (string, int, error)
}

// OAuthStartHandler starts a user-scoped OAuth authorization flow.
func OAuthStartHandler(deps OAuthHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.RequireUserID == nil || deps.PrepareRedirect == nil {
			http.Error(w, "oauth unavailable", http.StatusServiceUnavailable)
			return
		}
		userID, err := deps.RequireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			ServerID int64  `json:"serverId"`
			URL      string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		authURL, status, err := deps.PrepareRedirect(w, r, userID, OAuthStartRequest{ServerID: req.ServerID, URL: req.URL})
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"redirectUrl": authURL})
	}
}

// OAuthBootstrapHandler starts OAuth during unauthenticated setup.
func OAuthBootstrapHandler(deps OAuthHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.AuthEnabled != nil && deps.AuthEnabled() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if deps.PrepareRedirect == nil {
			http.Error(w, "oauth unavailable", http.StatusServiceUnavailable)
			return
		}
		serverID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("serverId")), 10, 64)
		if err != nil || serverID == 0 {
			http.Error(w, "serverId required", http.StatusBadRequest)
			return
		}
		authURL, status, err := deps.PrepareRedirect(w, r, deps.SystemUserID, OAuthStartRequest{ServerID: serverID})
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}
