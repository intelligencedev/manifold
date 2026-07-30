package mcpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"manifold/internal/config"
	"manifold/internal/persistence"
)

// OAuthCallbackState is the verified state restored from the OAuth temporary
// cookies. It is transport-only and therefore belongs with the MCP API
// surface rather than the application composition root.
type OAuthCallbackState struct {
	State           string
	StateCookieName string
	PKCECookieName  string
	PKCEVerifier    string
	TargetURL       string
	Resource        string
	UserID          int64
	ServerID        int64
}

// ReadOAuthCallbackState validates the state and PKCE cookies for a callback.
func ReadOAuthCallbackState(w http.ResponseWriter, r *http.Request, statePrefix, pkcePrefix string) (OAuthCallbackState, bool) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "state missing", http.StatusBadRequest)
		return OAuthCallbackState{}, false
	}
	callbackState := OAuthCallbackState{State: state, StateCookieName: OAuthCookieName(statePrefix, state), PKCECookieName: OAuthCookieName(pkcePrefix, state)}
	cookie, err := r.Cookie(callbackState.StateCookieName)
	if err != nil {
		http.Error(w, "state cookie missing", http.StatusBadRequest)
		return OAuthCallbackState{}, false
	}
	parts := strings.SplitN(cookie.Value, "|", 5)
	if len(parts) < 4 || parts[0] != state {
		http.Error(w, "invalid state cookie", http.StatusBadRequest)
		return OAuthCallbackState{}, false
	}
	pkceCookie, err := r.Cookie(callbackState.PKCECookieName)
	if err != nil || pkceCookie.Value == "" {
		http.Error(w, "pkce verifier missing", http.StatusBadRequest)
		return OAuthCallbackState{}, false
	}
	callbackState.TargetURL = parts[1]
	callbackState.Resource = parts[1]
	if len(parts) >= 5 && strings.TrimSpace(parts[4]) != "" {
		callbackState.Resource = strings.TrimSpace(parts[4])
	}
	callbackState.UserID, _ = strconv.ParseInt(parts[2], 10, 64)
	callbackState.ServerID, _ = strconv.ParseInt(parts[3], 10, 64)
	callbackState.PKCEVerifier = pkceCookie.Value
	return callbackState, true
}

func OAuthCookieName(prefix, state string) string {
	sum := sha256.Sum256([]byte(state))
	return prefix + base64.RawURLEncoding.EncodeToString(sum[:])
}

func UsesSecureCookies(r *http.Request) bool {
	return r != nil && (r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}

func ValidateOAuthTargetURL(targetURL string) (int, error) {
	if targetURL == "" {
		return http.StatusBadRequest, fmt.Errorf("url required")
	}
	u, err := url.Parse(targetURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return http.StatusBadRequest, fmt.Errorf("invalid url")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return http.StatusBadRequest, fmt.Errorf("unsupported url scheme")
	}
	if strings.Contains(u.Host, "..") {
		return http.StatusBadRequest, fmt.Errorf("invalid host")
	}
	return http.StatusOK, nil
}

// OAuthScopes chooses server-specific scopes, discovered scopes, or the
// conservative default used by the MCP OAuth flow.
func OAuthScopes(scopes []string, server *persistence.MCPServer) []string {
	if server != nil && len(server.OAuthScopes) > 0 {
		return server.OAuthScopes
	}
	if len(scopes) > 0 {
		return scopes
	}
	return []string{"openid", "profile"}
}

// RequiresOAuthPrompt identifies MCP authorization failures that should send
// the caller through the browser OAuth flow.
func RequiresOAuthPrompt(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") || strings.Contains(msg, "401") || strings.Contains(msg, "forbidden")
}

// ConvertToConfig maps a persisted MCP server into runtime configuration.
func ConvertToConfig(server persistence.MCPServer) config.MCPServerConfig {
	token := server.BearerToken
	if server.OAuthAccessToken != "" {
		token = server.OAuthAccessToken
	}
	return config.MCPServerConfig{
		Name:             server.Name,
		Command:          server.Command,
		Args:             server.Args,
		Env:              server.Env,
		URL:              server.URL,
		Headers:          server.Headers,
		Origin:           server.Origin,
		ProtocolVersion:  server.ProtocolVersion,
		KeepAliveSeconds: server.KeepAliveSeconds,
		BearerToken:      token,
	}
}
