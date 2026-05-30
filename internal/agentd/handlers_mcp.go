package agentd

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
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
	target, status, err := a.mcpOAuthRedirectTarget(r.Context(), userID, req)
	if err != nil {
		return "", status, err
	}
	prm, asm, status, err := a.mcpOAuthRedirectMetadata(r.Context(), target.url)
	if err != nil {
		return "", status, err
	}
	client, status, err := a.resolveMCPOAuthRedirectClient(r.Context(), userID, target.server, prm.ScopesSupported, asm)
	if err != nil {
		return "", status, err
	}
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("failed to generate PKCE")
	}
	state := uuid.New().String()
	a.setMCPOAuthRedirectCookies(w, r, state, target.url, userID, req.ServerID, verifier)
	authURL := client.configFor(a.cfg.Auth.RedirectURL, asm).AuthCodeURL(state, oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("resource", target.url),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	return authURL, http.StatusOK, nil
}

type mcpOAuthRedirectTarget struct {
	url    string
	server *persistence.MCPServer
}

type mcpOAuthRedirectClient struct {
	id     string
	secret string
	scopes []string
}

func (c mcpOAuthRedirectClient) configFor(redirectURL string, asm *authServerMeta) *oauth2.Config {
	redirectBase := computeBaseOrigin(redirectURL)
	return &oauth2.Config{
		ClientID:     c.id,
		ClientSecret: c.secret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  asm.AuthorizationEndpoint,
			TokenURL: asm.TokenEndpoint,
		},
		RedirectURL: redirectBase + "/api/mcp/oauth/callback",
		Scopes:      c.scopes,
	}
}

func (a *app) mcpOAuthRedirectTarget(ctx context.Context, userID int64, req mcpOAuthStartRequest) (mcpOAuthRedirectTarget, int, error) {
	targetURL := req.URL
	var server *persistence.MCPServer
	if req.ServerID != 0 {
		list, _ := a.mcpStore.List(ctx, userID)
		for _, s := range list {
			if s.ID == req.ServerID {
				targetURL = s.URL
				serverCopy := s
				server = &serverCopy
				break
			}
		}
	}
	if status, err := validateMCPOAuthTargetURL(targetURL); err != nil {
		return mcpOAuthRedirectTarget{}, status, err
	}
	return mcpOAuthRedirectTarget{url: targetURL, server: server}, http.StatusOK, nil
}

func validateMCPOAuthTargetURL(targetURL string) (int, error) {
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

func (a *app) mcpOAuthRedirectMetadata(ctx context.Context, targetURL string) (*oauthex.ProtectedResourceMetadata, *authServerMeta, int, error) {
	prm, err := a.discoverResourceMetadata(ctx, targetURL)
	if err != nil {
		return nil, nil, http.StatusBadGateway, fmt.Errorf("failed to discover resource metadata: %v", err)
	}
	if len(prm.AuthorizationServers) == 0 {
		return nil, nil, http.StatusBadGateway, fmt.Errorf("no authorization servers found for resource")
	}
	asm, err := a.discoverAuthServerMeta(ctx, prm.AuthorizationServers[0])
	if err != nil {
		return nil, nil, http.StatusBadGateway, fmt.Errorf("failed to discover auth server metadata: %v", err)
	}
	return prm, asm, http.StatusOK, nil
}

func (a *app) resolveMCPOAuthRedirectClient(
	ctx context.Context,
	userID int64,
	server *persistence.MCPServer,
	scopes []string,
	asm *authServerMeta,
) (mcpOAuthRedirectClient, int, error) {
	client := mcpOAuthRedirectClient{scopes: mcpOAuthScopes(scopes, server)}
	if server != nil {
		client.id = strings.TrimSpace(server.OAuthClientID)
		client.secret = strings.TrimSpace(server.OAuthClientSecret)
	}
	if client.id == "" {
		client.id = strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_ID"))
		client.secret = strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_SECRET"))
	}
	if client.id == "" && server != nil && asm.RegistrationEndpoint != "" {
		var status int
		var err error
		client, status, err = a.registerMCPOAuthRedirectClient(ctx, userID, server, client.scopes, asm.RegistrationEndpoint)
		if err != nil {
			return mcpOAuthRedirectClient{}, status, err
		}
	}
	if client.id == "" {
		return mcpOAuthRedirectClient{}, http.StatusBadRequest, fmt.Errorf("mcp oauth client id not configured for this server")
	}
	return client, http.StatusOK, nil
}

func mcpOAuthScopes(scopes []string, server *persistence.MCPServer) []string {
	if server != nil && len(server.OAuthScopes) > 0 {
		return server.OAuthScopes
	}
	if len(scopes) > 0 {
		return scopes
	}
	return []string{"openid", "profile"}
}

func (a *app) registerMCPOAuthRedirectClient(
	ctx context.Context,
	userID int64,
	server *persistence.MCPServer,
	scopes []string,
	registrationEndpoint string,
) (mcpOAuthRedirectClient, int, error) {
	redirectBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
	redirectURI := redirectBase + "/api/mcp/oauth/callback"
	clientID, clientSecret, err := a.registerOAuthClient(ctx, registrationEndpoint, server.Name, redirectURI, scopes)
	if err != nil {
		return mcpOAuthRedirectClient{}, http.StatusBadGateway, fmt.Errorf("dynamic registration failed: %v", err)
	}
	server.OAuthClientID = clientID
	server.OAuthClientSecret = clientSecret
	if saved, upErr := a.mcpStore.Upsert(ctx, userID, *server); upErr == nil {
		*server = saved
	}
	return mcpOAuthRedirectClient{id: clientID, secret: clientSecret, scopes: scopes}, http.StatusOK, nil
}

func (a *app) setMCPOAuthRedirectCookies(
	w http.ResponseWriter,
	r *http.Request,
	state string,
	targetURL string,
	userID int64,
	serverID int64,
	verifier string,
) {
	expiresAt := time.Now().Add(10 * time.Minute)
	secureCookies := mcpOAuthUsesSecureCookies(r)
	setMCPOAuthTempCookie(
		w,
		mcpOAuthCookieName(mcpOAuthStateCookiePrefix, state),
		fmt.Sprintf("%s|%s|%d|%d", state, targetURL, userID, serverID),
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
			Status:           "connected",
			HasToken:         s.BearerToken != "",
		})
	}

	// 3. Add DB servers
	for _, s := range dbServers {
		status := "connected"
		if s.Disabled {
			status = "disabled"
		} else if s.URL != "" && s.OAuthAccessToken != "" {
			if !s.OAuthExpiresAt.IsZero() && s.OAuthExpiresAt.Before(time.Now()) {
				// Token is expired - check if we can refresh
				if s.OAuthRefreshToken == "" {
					status = "needs_auth"
				}
				// If refresh token exists, we can try to refresh on next use
			}
		}
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
