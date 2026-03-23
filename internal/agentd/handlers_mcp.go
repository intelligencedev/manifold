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
	"regexp"
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
	OAuthClientID    string            `json:"oauthClientId,omitempty"`
	Source           string            `json:"source"` // "config" or "db"
	Status           string            `json:"status"` // "connected", "error", "needs_auth"
	HasToken         bool              `json:"hasToken"`
}

type mcpOAuthStartRequest struct {
	ServerID int64
	URL      string
}

const (
	mcpOAuthCookiePath        = "/api/mcp/oauth"
	mcpOAuthStateCookiePrefix = "mcp_oauth_state_"
	mcpOAuthPKCECookiePrefix  = "mcp_oauth_pkce_"
)

func mcpOAuthCookieName(prefix, state string) string {
	sum := sha256.Sum256([]byte(state))
	return prefix + base64.RawURLEncoding.EncodeToString(sum[:])
}

func mcpOAuthUsesSecureCookies(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setMCPOAuthTempCookie(w http.ResponseWriter, name, value string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     mcpOAuthCookiePath,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
}

func clearMCPOAuthTempCookie(w http.ResponseWriter, name string, secure bool) {
	setMCPOAuthTempCookie(w, name, "", time.Unix(0, 0), secure)
}

func requiresMCPOAuthPrompt(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") || strings.Contains(msg, "401") || strings.Contains(msg, "forbidden")
}

func (a *app) prepareMCPOAuthRedirect(w http.ResponseWriter, r *http.Request, userID int64, req mcpOAuthStartRequest) (string, int, error) {
	targetURL := req.URL
	var server *persistence.MCPServer
	if req.ServerID != 0 {
		list, _ := a.mcpStore.List(r.Context(), userID)
		for _, s := range list {
			if s.ID == req.ServerID {
				targetURL = s.URL
				ss := s
				server = &ss
				break
			}
		}
	}

	if targetURL == "" {
		return "", http.StatusBadRequest, fmt.Errorf("url required")
	}
	u, err := url.Parse(targetURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", http.StatusBadRequest, fmt.Errorf("invalid url")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", http.StatusBadRequest, fmt.Errorf("unsupported url scheme")
	}
	if strings.Contains(u.Host, "..") {
		return "", http.StatusBadRequest, fmt.Errorf("invalid host")
	}

	prm, err := a.discoverResourceMetadata(r.Context(), targetURL)
	if err != nil {
		return "", http.StatusBadGateway, fmt.Errorf("failed to discover resource metadata: %v", err)
	}
	if len(prm.AuthorizationServers) == 0 {
		return "", http.StatusBadGateway, fmt.Errorf("no authorization servers found for resource")
	}

	issuer := prm.AuthorizationServers[0]
	asm, err := a.discoverAuthServerMeta(r.Context(), issuer)
	if err != nil {
		return "", http.StatusBadGateway, fmt.Errorf("failed to discover auth server metadata: %v", err)
	}

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
	if clientID == "" && server != nil && asm.RegistrationEndpoint != "" {
		redirectBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
		redirectURI := redirectBase + "/api/mcp/oauth/callback"
		regScopes := prm.ScopesSupported
		clientIDReg, clientSecretReg, regErr := a.registerOAuthClient(r.Context(), asm.RegistrationEndpoint, server.Name, redirectURI, regScopes)
		if regErr != nil {
			return "", http.StatusBadGateway, fmt.Errorf("dynamic registration failed: %v", regErr)
		}
		server.OAuthClientID = clientIDReg
		server.OAuthClientSecret = clientSecretReg
		if saved, upErr := a.mcpStore.Upsert(r.Context(), userID, *server); upErr == nil {
			*server = saved
		}
		clientID = clientIDReg
		clientSecret = clientSecretReg
	}
	if clientID == "" {
		return "", http.StatusBadRequest, fmt.Errorf("mcp oauth client id not configured for this server")
	}

	scopes := prm.ScopesSupported
	if server != nil && len(server.OAuthScopes) > 0 {
		scopes = server.OAuthScopes
	}
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile"}
	}

	verifier, challenge, err := generatePKCE()
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("failed to generate PKCE")
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
	expiresAt := time.Now().Add(10 * time.Minute)
	secureCookies := mcpOAuthUsesSecureCookies(r)
	setMCPOAuthTempCookie(
		w,
		mcpOAuthCookieName(mcpOAuthStateCookiePrefix, state),
		fmt.Sprintf("%s|%s|%d|%d", state, targetURL, userID, req.ServerID),
		expiresAt,
		secureCookies,
	)
	setMCPOAuthTempCookie(
		w,
		mcpOAuthCookieName(mcpOAuthPKCECookiePrefix, state),
		verifier,
		expiresAt,
		secureCookies,
	)

	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("resource", targetURL),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	return authURL, http.StatusOK, nil
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

	// Build a set of names that have a DB entry so we can skip the config duplicate.
	dbNames := make(map[string]bool, len(dbServers))
	for _, s := range dbServers {
		dbNames[s.Name] = true
	}

	// 2. Add Config servers that are NOT already superseded by a DB entry.
	for _, s := range a.cfg.MCP.Servers {
		if dbNames[s.Name] {
			continue // DB entry will be shown instead
		}
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
			OAuthClientID:    s.OAuthClientID,
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

	// Only attempt connection if the server has a token or is local.
	// Remote servers without any token need OAuth first.
	if saved.URL != "" && saved.OAuthAccessToken == "" && saved.BearerToken == "" {
		fmt.Printf("MCP server %s: remote server needs OAuth, skipping initial connection\n", saved.Name)
	} else {
		cfgSrv, needsAuth, _ := a.refreshAndConvertToConfig(r.Context(), saved)
		if needsAuth {
			fmt.Printf("MCP server %s needs re-authentication (token expired)\n", saved.Name)
		} else if err := a.mcpManager.RegisterOne(r.Context(), a.baseToolRegistry, cfgSrv); err != nil {
			fmt.Printf("failed to connect to new MCP server: %v\n", err)
		}
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

	// Only attempt reconnection if the server has a token or is local.
	if saved.URL != "" && saved.OAuthAccessToken == "" && saved.BearerToken == "" {
		fmt.Printf("MCP server %s: remote server needs OAuth, skipping reconnection\n", saved.Name)
	} else {
		cfgSrv, needsAuth, _ := a.refreshAndConvertToConfig(r.Context(), saved)
		if needsAuth {
			fmt.Printf("MCP server %s needs re-authentication (token expired)\n", saved.Name)
		} else if err := a.mcpManager.RegisterOne(r.Context(), a.baseToolRegistry, cfgSrv); err != nil {
			fmt.Printf("failed to reconnect updated MCP server: %v\n", err)
		}
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

// seedConfigServersToDBIfMissing ensures that remote MCP servers defined in the
// config file also have a corresponding DB record (needed for OAuth token storage
// and the startup browser-auth prompt). Servers already in the DB (matched by
// name) are left unchanged.
func (a *app) seedConfigServersToDBIfMissing(ctx context.Context, userID int64, existing []persistence.MCPServer) {
	if a.mcpStore == nil {
		return
	}
	existingByName := make(map[string]bool, len(existing))
	for _, s := range existing {
		existingByName[s.Name] = true
	}
	for _, cs := range a.cfg.MCP.Servers {
		if cs.URL == "" || cs.BearerToken != "" {
			continue // local or already has static token — skip
		}
		if existingByName[cs.Name] {
			continue // already in DB
		}
		srv := persistence.MCPServer{
			Name:             cs.Name,
			URL:              cs.URL,
			Headers:          cs.Headers,
			Origin:           cs.Origin,
			ProtocolVersion:  cs.ProtocolVersion,
			KeepAliveSeconds: cs.KeepAliveSeconds,
		}
		if _, err := a.mcpStore.Upsert(ctx, userID, srv); err != nil {
			fmt.Printf("MCP server %s: failed to seed into DB: %v\n", cs.Name, err)
		} else {
			fmt.Printf("MCP server %s: seeded into DB for OAuth flow\n", cs.Name)
		}
	}
}

// RefreshMCPServersOnStartup refreshes OAuth tokens for all stored MCP servers
// that have expired tokens, and registers them with the MCP manager. This should
// be called after the app is initialized to restore OAuth-authenticated remote
// MCP servers on restart.
func (a *app) RefreshMCPServersOnStartup(ctx context.Context, userID int64) ([]int64, error) {
	if a.mcpStore == nil {
		return nil, nil
	}

	servers, err := a.mcpStore.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP servers: %w", err)
	}

	// Seed any config-file remote servers that aren't yet in the DB so they
	// can participate in the OAuth startup browser-auth prompt.
	a.seedConfigServersToDBIfMissing(ctx, userID, servers)

	// Re-list after potential seed so new records are included.
	servers, err = a.mcpStore.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to re-list MCP servers after seed: %w", err)
	}
	pendingAuthIDs := make([]int64, 0)

	for _, s := range servers {
		if s.Disabled {
			continue
		}

		if s.URL == "" {
			if err := a.mcpManager.RegisterOne(ctx, a.baseToolRegistry, convertToConfig(s)); err != nil {
				fmt.Printf("MCP server %s: registration failed: %v\n", s.Name, err)
			}
			continue
		}

		if s.OAuthAccessToken == "" && s.BearerToken == "" {
			// Remote server with no token — skip registration (will fail with
			// unauthorized/timeout) and mark as needing OAuth.
			fmt.Printf("MCP server %s: needs OAuth, deferring registration\n", s.Name)
			pendingAuthIDs = append(pendingAuthIDs, s.ID)
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
			pendingAuthIDs = append(pendingAuthIDs, s.ID)
			continue
		}

		// Token is valid (or was just refreshed), register the server
		if err := a.mcpManager.RegisterOne(ctx, a.baseToolRegistry, cfgSrv); err != nil {
			fmt.Printf("MCP server %s: registration failed: %v\n", s.Name, err)
		} else {
			fmt.Printf("MCP server %s: registered with cached/refreshed OAuth token\n", s.Name)
		}
	}

	return pendingAuthIDs, nil
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
			URL      string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		authURL, statusCode, err := a.prepareMCPOAuthRedirect(w, r, userID, mcpOAuthStartRequest{ServerID: req.ServerID, URL: req.URL})
		if err != nil {
			http.Error(w, err.Error(), statusCode)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"redirectUrl": authURL})
	}
}

func (a *app) mcpOAuthBootstrapHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if a.cfg.Auth.Enabled {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		serverID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("serverId")), 10, 64)
		if err != nil || serverID == 0 {
			http.Error(w, "serverId required", http.StatusBadRequest)
			return
		}
		authURL, statusCode, err := a.prepareMCPOAuthRedirect(w, r, systemUserID, mcpOAuthStartRequest{ServerID: serverID})
		if err != nil {
			http.Error(w, err.Error(), statusCode)
			return
		}
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

func (a *app) mcpOAuthCallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state == "" {
			http.Error(w, "state missing", http.StatusBadRequest)
			return
		}

		stateCookieName := mcpOAuthCookieName(mcpOAuthStateCookiePrefix, state)
		pkceCookieName := mcpOAuthCookieName(mcpOAuthPKCECookiePrefix, state)
		secureCookies := mcpOAuthUsesSecureCookies(r)
		defer clearMCPOAuthTempCookie(w, stateCookieName, secureCookies)
		defer clearMCPOAuthTempCookie(w, pkceCookieName, secureCookies)

		// 1. Verify state
		cookie, err := r.Cookie(stateCookieName)
		if err != nil {
			http.Error(w, "state cookie missing", http.StatusBadRequest)
			return
		}
		parts := strings.SplitN(cookie.Value, "|", 4)
		if len(parts) < 4 {
			http.Error(w, "invalid state cookie", http.StatusBadRequest)
			return
		}
		expectedState, targetURL := parts[0], parts[1]
		userIDStr := parts[2]
		var serverID int64
		if v, err := strconv.ParseInt(parts[3], 10, 64); err == nil {
			serverID = v
		}

		// Load PKCE verifier
		pkceCookie, err := r.Cookie(pkceCookieName)
		if err != nil || pkceCookie.Value == "" {
			http.Error(w, "pkce verifier missing", http.StatusBadRequest)
			return
		}

		if state != expectedState {
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
						a.mcpManager.RemoveOne(sc.Name, a.baseToolRegistry)
						if err := a.mcpManager.RegisterOne(ctx, a.baseToolRegistry, convertToConfig(sc)); err != nil {
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

// resourceMetadataRE extracts the resource_metadata URL value from a Bearer WWW-Authenticate challenge.
var resourceMetadataRE = regexp.MustCompile(`(?i)\bresource_metadata\s*=\s*"([^"]+)"`)

func extractResourceMetadataURL(header string) string {
	m := resourceMetadataRE.FindStringSubmatch(header)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// discoverResourceMetadata resolves the protected-resource metadata for resourceURL using
// a three-strategy fallback to handle non-standard server deployments:
//
//  1. RFC 9728 §3.3 — probe the resource URL, parse WWW-Authenticate for resource_metadata param
//  2. RFC 9728 §3   — construct /.well-known/oauth-protected-resource{path} directly
//  3. Fallback       — hit host-level /.well-known/oauth-authorization-server and synthesise metadata
func (a *app) discoverResourceMetadata(ctx context.Context, resourceURL string) (*oauthex.ProtectedResourceMetadata, error) {
	u, err := url.Parse(resourceURL)
	if err != nil {
		return nil, err
	}

	// Strategy 1: probe the resource URL and extract resource_metadata from WWW-Authenticate.
	if meta, _ := a.discoverResourceMetadataFromChallenge(ctx, resourceURL); meta != nil {
		return meta, nil
	}

	// Strategy 2: RFC 9728 §3 well-known path.
	wellKnown := fmt.Sprintf("%s://%s/.well-known/oauth-protected-resource%s", u.Scheme, u.Host, u.Path)
	req, _ := http.NewRequestWithContext(ctx, "GET", wellKnown, nil)
	if resp, err2 := a.httpClient.Do(req); err2 == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var meta oauthex.ProtectedResourceMetadata
			if err3 := json.NewDecoder(resp.Body).Decode(&meta); err3 == nil {
				return &meta, nil
			}
		}
	}

	// Strategy 3: host-level /.well-known/oauth-authorization-server — synthesise metadata.
	asMetaURL := fmt.Sprintf("%s://%s/.well-known/oauth-authorization-server", u.Scheme, u.Host)
	req2, _ := http.NewRequestWithContext(ctx, "GET", asMetaURL, nil)
	resp2, err2 := a.httpClient.Do(req2)
	if err2 != nil {
		return nil, fmt.Errorf("failed to discover resource metadata: no well-known endpoint responded")
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to discover resource metadata: status %d from auth-server well-known", resp2.StatusCode)
	}
	var asMeta struct {
		Issuer string `json:"issuer"`
	}
	if err3 := json.NewDecoder(resp2.Body).Decode(&asMeta); err3 != nil || asMeta.Issuer == "" {
		return nil, fmt.Errorf("failed to discover resource metadata: no issuer in auth-server well-known")
	}
	return &oauthex.ProtectedResourceMetadata{
		Resource:             resourceURL,
		AuthorizationServers: []string{asMeta.Issuer},
	}, nil
}

// discoverResourceMetadataFromChallenge probes resourceURL unauthenticated and extracts the
// resource_metadata URL from any WWW-Authenticate Bearer challenge (RFC 9728 §3.3).
func (a *app) discoverResourceMetadataFromChallenge(ctx context.Context, resourceURL string) (*oauthex.ProtectedResourceMetadata, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", resourceURL, nil)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return nil, nil
	}
	for _, h := range resp.Header.Values("WWW-Authenticate") {
		metaURL := extractResourceMetadataURL(h)
		if metaURL == "" {
			continue
		}
		req2, _ := http.NewRequestWithContext(ctx, "GET", metaURL, nil)
		resp2, err2 := a.httpClient.Do(req2)
		if err2 != nil {
			return nil, err2
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			// Metadata URL exists but is broken; signal failure so caller falls through.
			return nil, fmt.Errorf("resource_metadata URL %s returned %d", metaURL, resp2.StatusCode)
		}
		var meta oauthex.ProtectedResourceMetadata
		if err3 := json.NewDecoder(resp2.Body).Decode(&meta); err3 != nil {
			return nil, err3
		}
		return &meta, nil
	}
	return nil, nil
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
