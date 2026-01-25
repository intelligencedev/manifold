package agentd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	oauthex "github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"manifold/internal/config"
	"manifold/internal/persistence"
)

type mcpServerResponse struct {
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
	Source           string            `json:"source"` // "config" or "db"
	Status           string            `json:"status"` // "connected", "error", "needs_auth"
	HasToken         bool              `json:"hasToken"`
}

func (a *app) mcpServersHandler() http.HandlerFunc {
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
			a.handleListMCPServers(w, r, userID)
		case http.MethodPost:
			a.handleCreateMCPServer(w, r, userID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) mcpServerDetailHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/mcp/servers/")
		name = strings.TrimSpace(name)
		if name == "" {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodPut:
			a.handleUpdateMCPServer(w, r, userID, name)
		case http.MethodDelete:
			a.handleDeleteMCPServer(w, r, userID, name)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *app) handleListMCPServers(w http.ResponseWriter, r *http.Request, userID int64) {
	// 1. Get DB servers
	dbServers, err := a.mcpStore.List(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	out := make([]mcpServerResponse, 0)

	// 2. Add Config servers (read-only view)
	for _, s := range a.cfg.MCP.Servers {
		out = append(out, mcpServerResponse{
			Name:             s.Name,
			Command:          s.Command,
			Args:             s.Args,
			Env:              s.Env,
			URL:              s.URL,
			Headers:          s.Headers,
			Origin:           s.Origin,
			ProtocolVersion:  s.ProtocolVersion,
			KeepAliveSeconds: s.KeepAliveSeconds,
			Source:           "config",
			Status:           "connected", // TODO: check actual status from manager
			HasToken:         s.BearerToken != "",
		})
	}

	// 3. Add DB servers
	for _, s := range dbServers {
		status := "connected"
		if s.Disabled {
			status = "disabled"
		} else if s.URL != "" && s.OAuthAccessToken != "" {
			// Check if OAuth token is expired and needs re-auth
			if !s.OAuthExpiresAt.IsZero() && s.OAuthExpiresAt.Before(time.Now()) {
				// Token is expired - check if we can refresh
				if s.OAuthRefreshToken == "" {
					status = "needs_auth"
				}
				// If refresh token exists, we can try to refresh on next use
			}
		}
		// TODO: Check if manager has active session

		out = append(out, mcpServerResponse{
			ID:               s.ID,
			Name:             s.Name,
			Command:          s.Command,
			Args:             s.Args,
			Env:              s.Env,
			URL:              s.URL,
			Headers:          s.Headers,
			Origin:           s.Origin,
			ProtocolVersion:  s.ProtocolVersion,
			KeepAliveSeconds: s.KeepAliveSeconds,
			Disabled:         s.Disabled,
			Source:           "db",
			Status:           status,
			HasToken:         s.OAuthAccessToken != "" || s.BearerToken != "",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (a *app) handleCreateMCPServer(w http.ResponseWriter, r *http.Request, userID int64) {
	var req persistence.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	saved, err := a.mcpStore.Upsert(r.Context(), userID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh token if needed and trigger connection
	cfgSrv, needsAuth, _ := a.refreshAndConvertToConfig(r.Context(), saved)
	if needsAuth {
		// Token expired and could not be refreshed - still create server but mark as needing auth
		fmt.Printf("MCP server %s needs re-authentication (token expired)\n", saved.Name)
	} else if err := a.mcpManager.RegisterOne(r.Context(), a.baseToolRegistry, cfgSrv); err != nil {
		// Log error but don't fail the request
		fmt.Printf("failed to connect to new MCP server: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(saved)
}

func (a *app) handleUpdateMCPServer(w http.ResponseWriter, r *http.Request, userID int64, name string) {
	var req persistence.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Name = name // Force name from URL

	saved, err := a.mcpStore.Upsert(r.Context(), userID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh token if needed and reconnect
	cfgSrv, needsAuth, _ := a.refreshAndConvertToConfig(r.Context(), saved)
	if needsAuth {
		fmt.Printf("MCP server %s needs re-authentication (token expired)\n", saved.Name)
	} else if err := a.mcpManager.RegisterOne(r.Context(), a.baseToolRegistry, cfgSrv); err != nil {
		fmt.Printf("failed to reconnect updated MCP server: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(saved)
}

func (a *app) handleDeleteMCPServer(w http.ResponseWriter, r *http.Request, userID int64, name string) {
	if err := a.mcpStore.Delete(r.Context(), userID, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Disconnect
	a.mcpManager.RemoveOne(name, a.baseToolRegistry)

	w.WriteHeader(http.StatusNoContent)
}

func convertToConfig(s persistence.MCPServer) config.MCPServerConfig {
	token := s.BearerToken
	if s.OAuthAccessToken != "" {
		token = s.OAuthAccessToken
	}
	return config.MCPServerConfig{
		Name:             s.Name,
		Command:          s.Command,
		Args:             s.Args,
		Env:              s.Env,
		URL:              s.URL,
		Headers:          s.Headers,
		Origin:           s.Origin,
		ProtocolVersion:  s.ProtocolVersion,
		KeepAliveSeconds: s.KeepAliveSeconds,
		BearerToken:      token,
	}
}

// refreshAndConvertToConfig refreshes expired OAuth tokens if needed and converts
// the server to a config object. Returns the config, whether re-auth is needed,
// and any error encountered during refresh.
func (a *app) refreshAndConvertToConfig(ctx context.Context, s persistence.MCPServer) (config.MCPServerConfig, bool, error) {
	// Attempt to refresh token if needed
	refreshed, needsAuth, err := a.refreshOAuthTokenIfNeeded(ctx, s)
	if err != nil {
		// Log but continue with whatever token state we have
		fmt.Printf("mcp oauth token refresh warning for %s: %v\n", s.Name, err)
	}
	return convertToConfig(refreshed), needsAuth, nil
}

// RefreshMCPServersOnStartup refreshes OAuth tokens for all stored MCP servers
// that have expired tokens, and registers them with the MCP manager. This should
// be called after the app is initialized to restore OAuth-authenticated remote
// MCP servers on restart.
func (a *app) RefreshMCPServersOnStartup(ctx context.Context, userID int64) error {
	if a.mcpStore == nil {
		return nil
	}

	servers, err := a.mcpStore.List(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	for _, s := range servers {
		if s.Disabled {
			continue
		}

		// Skip servers without OAuth tokens (they don't need refresh)
		if s.OAuthAccessToken == "" && s.BearerToken == "" {
			continue
		}

		// Skip local command-based servers (no OAuth flow needed)
		if s.URL == "" {
			continue
		}

		// Refresh token if needed and register
		cfgSrv, needsAuth, err := a.refreshAndConvertToConfig(ctx, s)
		if err != nil {
			fmt.Printf("MCP server %s: token refresh error: %v\n", s.Name, err)
			continue
		}

		if needsAuth {
			fmt.Printf("MCP server %s: OAuth token expired, needs re-authentication\n", s.Name)
			continue
		}

		// Token is valid (or was just refreshed), register the server
		if err := a.mcpManager.RegisterOne(ctx, a.baseToolRegistry, cfgSrv); err != nil {
			fmt.Printf("MCP server %s: registration failed: %v\n", s.Name, err)
		} else {
			fmt.Printf("MCP server %s: registered with cached/refreshed OAuth token\n", s.Name)
		}
	}

	return nil
}

// OAuth Handlers

func (a *app) mcpOAuthStartHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req struct {
			ServerID int64  `json:"serverId"`
			URL      string `json:"url"` // If new server
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		targetURL := req.URL
		var server *persistence.MCPServer
		if req.ServerID != 0 {
			// Try to find by ID (iterating since we only have List/GetByName)
			list, _ := a.mcpStore.List(r.Context(), userID)
			for _, s := range list {
				if s.ID == req.ServerID {
					targetURL = s.URL
					// capture for later use
					ss := s
					server = &ss
					break
				}
			}
		}

		if targetURL == "" {
			http.Error(w, "url required", http.StatusBadRequest)
			return
		}
		// Validate URL to avoid SSRF / invalid schemes
		u, err := url.Parse(targetURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			http.Error(w, "invalid url", http.StatusBadRequest)
			return
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			http.Error(w, "unsupported url scheme", http.StatusBadRequest)
			return
		}
		if strings.Contains(u.Host, "..") {
			http.Error(w, "invalid host", http.StatusBadRequest)
			return
		}

		// 1. Discover Protected Resource Metadata
		prm, err := a.discoverResourceMetadata(r.Context(), targetURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to discover resource metadata: %v", err), http.StatusBadGateway)
			return
		}
		if len(prm.AuthorizationServers) == 0 {
			http.Error(w, "no authorization servers found for resource", http.StatusBadGateway)
			return
		}

		// 2. Discover Auth Server Metadata
		issuer := prm.AuthorizationServers[0] // Pick first
		asm, err := a.discoverAuthServerMeta(r.Context(), issuer)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to discover auth server metadata: %v", err), http.StatusBadGateway)
			return
		}

		// 3. Determine client ID. Preference order:
		//    a) Stored per-server OAuthClientID
		//    b) Environment MCP_OAUTH_CLIENT_ID
		//    c) Dynamic registration (if supported and no ID yet)
		clientID := ""
		clientSecret := ""
		if server != nil {
			clientID = strings.TrimSpace(server.OAuthClientID)
			clientSecret = strings.TrimSpace(server.OAuthClientSecret)
		}
		if clientID == "" {
			clientID = strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_ID"))
			clientSecret = strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_SECRET"))
		}
		// Attempt dynamic client registration if still empty and registration endpoint advertised
		if clientID == "" && server != nil && asm.RegistrationEndpoint != "" {
			// Derive canonical redirect URI (base origin + callback path) so registration matches later auth usage.
			redirectBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
			redirectURI := redirectBase + "/api/mcp/oauth/callback"
			// Scopes for registration can leverage resource metadata (fallback later)
			regScopes := prm.ScopesSupported
			clientIDReg, clientSecretReg, regErr := a.registerOAuthClient(r.Context(), asm.RegistrationEndpoint, server.Name, redirectURI, regScopes)
			if regErr != nil {
				http.Error(w, fmt.Sprintf("dynamic registration failed: %v", regErr), http.StatusBadGateway)
				return
			}
			server.OAuthClientID = clientIDReg
			server.OAuthClientSecret = clientSecretReg
			// Persist updated server credentials
			if saved, upErr := a.mcpStore.Upsert(r.Context(), userID, *server); upErr == nil {
				*server = saved
			}
			clientID = clientIDReg
			clientSecret = clientSecretReg
		}
		if clientID == "" {
			http.Error(w, "mcp oauth client id not configured for this server", http.StatusBadRequest)
			return
		}

		// 4. Determine scopes: prefer stored server OAuthScopes if serverId provided; else resource metadata scopes; else defaults.
		scopes := prm.ScopesSupported
		if server != nil && len(server.OAuthScopes) > 0 {
			scopes = server.OAuthScopes
		}
		if len(scopes) == 0 {
			// Provide sensible defaults; HuggingFace expects standard OIDC scopes.
			scopes = []string{"openid", "profile"}
		}

		// PKCE
		verifier, challenge, err := generatePKCE()
		if err != nil {
			http.Error(w, "failed to generate PKCE", http.StatusInternalServerError)
			return
		}

		redirectBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
		conf := &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  asm.AuthorizationEndpoint,
				TokenURL: asm.TokenEndpoint,
			},
			RedirectURL: redirectBase + "/api/mcp/oauth/callback",
			Scopes:      scopes,
		}

		state := uuid.New().String()
		// Store state + context (userID, serverURL, serverID) in cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "mcp_oauth_state",
			Value:    fmt.Sprintf("%s|%s|%d|%d", state, targetURL, userID, req.ServerID),
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			Expires:  time.Now().Add(10 * time.Minute),
		})

		// Store PKCE verifier in HttpOnly cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "mcp_oauth_pkce",
			Value:    verifier,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			Expires:  time.Now().Add(10 * time.Minute),
		})

		authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline,
			oauth2.SetAuthURLParam("resource", targetURL),
			oauth2.SetAuthURLParam("code_challenge", challenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)

		json.NewEncoder(w).Encode(map[string]string{"redirectUrl": authURL})
	}
}

func (a *app) mcpOAuthCallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Verify state
		cookie, err := r.Cookie("mcp_oauth_state")
		if err != nil {
			http.Error(w, "state cookie missing", http.StatusBadRequest)
			return
		}
		parts := strings.SplitN(cookie.Value, "|", 4)
		if len(parts) < 3 {
			http.Error(w, "invalid state cookie", http.StatusBadRequest)
			return
		}
		expectedState, targetURL := parts[0], parts[1]
		userIDStr := parts[2]
		var serverID int64
		if len(parts) >= 4 {
			if v, err := strconv.ParseInt(parts[3], 10, 64); err == nil {
				serverID = v
			}
		}

		// Load PKCE verifier
		pkceCookie, err := r.Cookie("mcp_oauth_pkce")
		if err != nil || pkceCookie.Value == "" {
			http.Error(w, "pkce verifier missing", http.StatusBadRequest)
			return
		}

		if r.URL.Query().Get("state") != expectedState {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "code missing", http.StatusBadRequest)
			return
		}

		// 2. Exchange code
		prm, err := a.discoverResourceMetadata(r.Context(), targetURL)
		if err != nil {
			http.Error(w, "metadata rediscovery failed", http.StatusBadGateway)
			return
		}
		issuer := prm.AuthorizationServers[0]
		asm, err := a.discoverAuthServerMeta(r.Context(), issuer)
		if err != nil {
			http.Error(w, "auth meta rediscovery failed", http.StatusBadGateway)
			return
		}

		// Resolve per-server clientID
		clientID := ""
		clientSecret := ""
		if serverID != 0 {
			// Load servers for the same user
			uid, _ := strconv.ParseInt(userIDStr, 10, 64)
			list, _ := a.mcpStore.List(r.Context(), uid)
			for _, s := range list {
				if s.ID == serverID {
					clientID = strings.TrimSpace(s.OAuthClientID)
					clientSecret = strings.TrimSpace(s.OAuthClientSecret)
					break
				}
			}
		}
		if clientID == "" {
			clientID = strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_ID"))
			clientSecret = strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_SECRET"))
		}
		if clientID == "" {
			http.Error(w, "mcp oauth client id not configured for this server", http.StatusBadRequest)
			return
		}
		redirectBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
		conf := &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  asm.AuthorizationEndpoint,
				TokenURL: asm.TokenEndpoint,
			},
			RedirectURL: redirectBase + "/api/mcp/oauth/callback",
		}

		// Use a context with custom HTTP client so we can inject test doubles
		ctx := context.WithValue(r.Context(), oauth2.HTTPClient, a.httpClient)
		token, err := conf.Exchange(ctx, code,
			oauth2.SetAuthURLParam("code_verifier", pkceCookie.Value),
			oauth2.SetAuthURLParam("resource", targetURL),
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("token exchange failed: %v", err), http.StatusInternalServerError)
			return
		}

		// 3. Persist token if serverID known
		if serverID != 0 {
			uid, _ := strconv.ParseInt(userIDStr, 10, 64)
			list, _ := a.mcpStore.List(r.Context(), uid)
			for _, s := range list {
				if s.ID == serverID {
					s.OAuthAccessToken = token.AccessToken
					s.OAuthRefreshToken = token.RefreshToken
					s.OAuthExpiresAt = token.Expiry
					if _, err := a.mcpStore.Upsert(r.Context(), s.UserID, s); err != nil {
						http.Error(w, "failed to persist token", http.StatusInternalServerError)
						return
					}
					// Hot-reload server tools with new token (async to avoid delaying user response)
					serverCopy := s
					go func(sc persistence.MCPServer) {
						// Best-effort re-registration; errors logged but not surfaced to user.
						ctx := context.Background()
						a.mcpManager.RemoveOne(sc.Name, a.toolRegistry)
						if err := a.mcpManager.RegisterOne(ctx, a.toolRegistry, convertToConfig(sc)); err != nil {
							fmt.Printf("mcp oauth re-register failed for %s: %v\n", sc.Name, err)
						}
					}(serverCopy)
					break
				}
			}
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
<html><body>
<h1>Connected!</h1>
<p>Token received. You can close this window.</p>
<script>
window.opener.postMessage({
	type: 'mcp-oauth-success', 
	token: '%s',
	refreshToken: '%s',
	expiry: '%s',
	url: '%s'
}, '*');
window.close();
</script>
</body></html>`, token.AccessToken, token.RefreshToken, token.Expiry.Format(time.RFC3339), targetURL)
	}
}

// Discovery Helpers

type authServerMeta struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

func (a *app) discoverResourceMetadata(ctx context.Context, resourceURL string) (*oauthex.ProtectedResourceMetadata, error) {
	u, err := url.Parse(resourceURL)
	if err != nil {
		return nil, err
	}
	// Append /.well-known/oauth-protected-resource/{path}
	// RFC 9728 logic simplified
	wellKnown := fmt.Sprintf("%s://%s/.well-known/oauth-protected-resource%s", u.Scheme, u.Host, u.Path)

	req, _ := http.NewRequestWithContext(ctx, "GET", wellKnown, nil)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var meta oauthex.ProtectedResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (a *app) discoverAuthServerMeta(ctx context.Context, issuer string) (*authServerMeta, error) {
	// RFC 8414 + OpenID Connect Discovery compliant metadata discovery.
	// Handles issuers with path components (e.g., Keycloak realms, Okta tenants).
	u, err := url.Parse(issuer)
	if err != nil {
		return nil, err
	}

	// Preserve the path component, trimming any trailing slash
	path := strings.TrimSuffix(u.Path, "/")

	// Build candidates per RFC 8414 §3 and OIDC Discovery:
	// 1. RFC 8414 OAuth AS metadata (host/path/.well-known/oauth-authorization-server)
	// 2. Legacy OIDC Discovery (host/path/.well-known/openid-configuration)
	// 3. RFC 8414 OpenID config (host/.well-known/openid-configuration) - fallback for root
	candidates := []string{
		fmt.Sprintf("%s://%s%s/.well-known/oauth-authorization-server", u.Scheme, u.Host, path),
		fmt.Sprintf("%s://%s%s/.well-known/openid-configuration", u.Scheme, u.Host, path),
		fmt.Sprintf("%s://%s/.well-known/openid-configuration", u.Scheme, u.Host),
	}

	var lastErr error
	for _, metaURL := range candidates {
		req, _ := http.NewRequestWithContext(ctx, "GET", metaURL, nil)
		resp, err := a.httpClient.Do(req)
		if err != nil {
			if resp != nil {
				resp.Body.Close()
			}
			lastErr = fmt.Errorf("%s: %v", metaURL, err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var meta authServerMeta
			decErr := json.NewDecoder(resp.Body).Decode(&meta)
			resp.Body.Close()

			if decErr == nil {
				// Note: Per RFC 8414 §3.3, issuer validation is optional and should be lenient.
				// Many real-world providers use different domains/paths between the advertised
				// authorization_servers entry and the actual issuer claim in the metadata.
				// We skip strict validation to maximize compatibility.
				return &meta, nil
			}
			lastErr = fmt.Errorf("%s: decode error: %v", metaURL, decErr)
		} else {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s: status %d", metaURL, resp.StatusCode)
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("metadata not found (last attempt: %v)", lastErr)
	}
	return nil, fmt.Errorf("metadata not found")
}

// PKCE helpers
func generatePKCE() (verifier string, challenge string, err error) {
	// Generate a random 32-byte value and encode URL-safe (no padding)
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// registerOAuthClient performs OAuth 2.0 Dynamic Client Registration (RFC 7591)
// against the authorization server registration endpoint. Returns client_id
// and optional client_secret.
func (a *app) registerOAuthClient(ctx context.Context, registrationEndpoint, clientName, redirectURI string, scopes []string) (clientID, clientSecret string, err error) {
	body := map[string]any{
		"client_name":                clientName,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"redirect_uris":              []string{redirectURI},
		"token_endpoint_auth_method": "none",
	}
	if len(scopes) > 0 {
		body["scope"] = strings.Join(scopes, " ")
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, bytes.NewReader(b))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("registration status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	return out.ClientID, out.ClientSecret, nil
}

// computeBaseOrigin derives the base origin (scheme://host[:port]) from a configured
// redirect URL that may itself include a path like /auth/callback. If parsing fails,
// the input is returned unchanged.
func computeBaseOrigin(full string) string {
	u, err := url.Parse(full)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return full
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

// refreshOAuthTokenIfNeeded checks if the server's OAuth access token is expired or about to expire,
// and attempts to refresh it using the stored refresh token. Returns the (possibly updated) server
// record and a boolean indicating whether re-authentication is required.
//
// Token refresh logic:
// - If no OAuth access token is set, returns the server unchanged
// - If the token has no expiry time set, returns the server unchanged (treat as valid)
// - If the token expires within the next 5 minutes, attempt refresh
// - On successful refresh, updates the database and returns the new server record
// - On refresh failure, clears the access token and marks server as needing re-auth
func (a *app) refreshOAuthTokenIfNeeded(ctx context.Context, srv persistence.MCPServer) (persistence.MCPServer, bool, error) {
	// No OAuth token set - nothing to refresh
	if srv.OAuthAccessToken == "" {
		return srv, false, nil
	}

	// No expiry set - treat token as valid (some providers don't set expiry)
	if srv.OAuthExpiresAt.IsZero() {
		return srv, false, nil
	}

	// Check if token expires within the next 5 minutes
	refreshThreshold := time.Now().Add(5 * time.Minute)
	if srv.OAuthExpiresAt.After(refreshThreshold) {
		// Token still valid
		return srv, false, nil
	}

	// Token expired or expiring soon - need to refresh
	if srv.OAuthRefreshToken == "" {
		// No refresh token available - require re-authentication
		srv.OAuthAccessToken = ""
		srv.OAuthExpiresAt = time.Time{}
		_, _ = a.mcpStore.Upsert(ctx, srv.UserID, srv)
		return srv, true, nil
	}

	// Attempt to refresh the token
	newToken, err := a.performTokenRefresh(ctx, srv)
	if err != nil {
		// Refresh failed - clear tokens and require re-auth
		srv.OAuthAccessToken = ""
		srv.OAuthRefreshToken = ""
		srv.OAuthExpiresAt = time.Time{}
		_, _ = a.mcpStore.Upsert(ctx, srv.UserID, srv)
		return srv, true, fmt.Errorf("token refresh failed: %w", err)
	}

	// Update server with new tokens
	srv.OAuthAccessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		srv.OAuthRefreshToken = newToken.RefreshToken
	}
	srv.OAuthExpiresAt = newToken.Expiry

	// Persist updated tokens
	updated, err := a.mcpStore.Upsert(ctx, srv.UserID, srv)
	if err != nil {
		return srv, false, fmt.Errorf("failed to persist refreshed token: %w", err)
	}

	return updated, false, nil
}

// performTokenRefresh exchanges a refresh token for a new access token using the
// authorization server's token endpoint.
func (a *app) performTokenRefresh(ctx context.Context, srv persistence.MCPServer) (*oauth2.Token, error) {
	if srv.URL == "" {
		return nil, fmt.Errorf("server URL required for token refresh")
	}

	// Discover authorization server metadata
	prm, err := a.discoverResourceMetadata(ctx, srv.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover resource metadata: %w", err)
	}
	if len(prm.AuthorizationServers) == 0 {
		return nil, fmt.Errorf("no authorization servers found for resource")
	}

	issuer := prm.AuthorizationServers[0]
	asm, err := a.discoverAuthServerMeta(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to discover auth server metadata: %w", err)
	}

	// Build token refresh request
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {srv.OAuthRefreshToken},
		"client_id":     {srv.OAuthClientID},
	}
	if srv.OAuthClientSecret != "" {
		data.Set("client_secret", srv.OAuthClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, asm.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in refresh response")
	}

	token := &oauth2.Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
	}
	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}
