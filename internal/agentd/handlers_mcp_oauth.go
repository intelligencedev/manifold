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

	oauthex "github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"manifold/internal/persistence"
)

var (
	authMethodClientSecretBasic = oauthMetadataToken("client", "secret", "basic")
	authMethodClientSecretPost  = oauthMetadataToken("client", "secret", "post")
	authMethodNone              = "none"

	authServerTokenAuthMethodsKey        = oauthMetadataToken("token", "endpoint", "auth", "methods", "supported")
	clientRegistrationTokenAuthMethodKey = oauthMetadataToken("token", "endpoint", "auth", "method")
)

func oauthMetadataToken(parts ...string) string {
	return strings.Join(parts, "_")
}

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
		callbackState, ok := readMCPOAuthCallbackState(w, r)
		if !ok {
			return
		}
		secureCookies := mcpOAuthUsesSecureCookies(r)
		defer clearMCPOAuthTempCookie(w, callbackState.stateCookieName, secureCookies)
		defer clearMCPOAuthTempCookie(w, callbackState.pkceCookieName, secureCookies)

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "code missing", http.StatusBadRequest)
			return
		}
		token, err := a.exchangeMCPOAuthCallbackCode(r, callbackState, code)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "client id not configured") {
				status = http.StatusBadRequest
			} else if strings.Contains(err.Error(), "rediscovery failed") {
				status = http.StatusBadGateway
			}
			http.Error(w, err.Error(), status)
			return
		}
		if err := a.persistMCPOAuthCallbackToken(r.Context(), callbackState, token); err != nil {
			http.Error(w, "failed to persist token", http.StatusInternalServerError)
			return
		}
		writeMCPOAuthCallbackSuccess(w, token, callbackState.targetURL)
	}
}

type mcpOAuthCallbackState struct {
	state           string
	stateCookieName string
	pkceCookieName  string
	pkceVerifier    string
	targetURL       string
	resource        string
	userID          int64
	serverID        int64
}

func readMCPOAuthCallbackState(w http.ResponseWriter, r *http.Request) (mcpOAuthCallbackState, bool) {
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "state missing", http.StatusBadRequest)
		return mcpOAuthCallbackState{}, false
	}
	callbackState := mcpOAuthCallbackState{
		state:           state,
		stateCookieName: mcpOAuthCookieName(mcpOAuthStateCookiePrefix, state),
		pkceCookieName:  mcpOAuthCookieName(mcpOAuthPKCECookiePrefix, state),
	}
	if !populateMCPOAuthStateCookie(w, r, &callbackState) {
		return mcpOAuthCallbackState{}, false
	}
	if !populateMCPOAuthPKCECookie(w, r, &callbackState) {
		return mcpOAuthCallbackState{}, false
	}
	return callbackState, true
}

func populateMCPOAuthStateCookie(w http.ResponseWriter, r *http.Request, callbackState *mcpOAuthCallbackState) bool {
	cookie, err := r.Cookie(callbackState.stateCookieName)
	if err != nil {
		http.Error(w, "state cookie missing", http.StatusBadRequest)
		return false
	}
	parts := strings.SplitN(cookie.Value, "|", 5)
	if len(parts) < 4 || parts[0] != callbackState.state {
		http.Error(w, "invalid state cookie", http.StatusBadRequest)
		return false
	}
	userID, _ := strconv.ParseInt(parts[2], 10, 64)
	serverID, _ := strconv.ParseInt(parts[3], 10, 64)
	callbackState.targetURL = parts[1]
	callbackState.resource = parts[1]
	if len(parts) >= 5 && strings.TrimSpace(parts[4]) != "" {
		callbackState.resource = strings.TrimSpace(parts[4])
	}
	callbackState.userID = userID
	callbackState.serverID = serverID
	return true
}

func populateMCPOAuthPKCECookie(w http.ResponseWriter, r *http.Request, callbackState *mcpOAuthCallbackState) bool {
	pkceCookie, err := r.Cookie(callbackState.pkceCookieName)
	if err != nil || pkceCookie.Value == "" {
		http.Error(w, "pkce verifier missing", http.StatusBadRequest)
		return false
	}
	callbackState.pkceVerifier = pkceCookie.Value
	return true
}

func (a *app) exchangeMCPOAuthCallbackCode(
	r *http.Request,
	callbackState mcpOAuthCallbackState,
	code string,
) (*oauth2.Token, error) {
	prm, err := a.discoverResourceMetadata(r.Context(), callbackState.targetURL)
	if err != nil {
		return nil, fmt.Errorf("metadata rediscovery failed")
	}
	asm, err := a.discoverAuthServerMeta(r.Context(), prm.AuthorizationServers[0])
	if err != nil {
		return nil, fmt.Errorf("auth meta rediscovery failed")
	}
	conf, err := a.mcpOAuthCallbackConfig(r.Context(), callbackState, asm)
	if err != nil {
		return nil, err
	}
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, a.httpClient)
	token, err := conf.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", callbackState.pkceVerifier),
		oauth2.SetAuthURLParam("resource", callbackState.resource),
	)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %v", err)
	}
	return token, nil
}

func (a *app) mcpOAuthCallbackConfig(
	ctx context.Context,
	callbackState mcpOAuthCallbackState,
	asm *authServerMeta,
) (*oauth2.Config, error) {
	clientID, clientSecret := a.mcpOAuthCallbackClient(ctx, callbackState)
	if clientID == "" {
		return nil, fmt.Errorf("mcp oauth client id not configured for this server")
	}
	redirectBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   asm.AuthorizationEndpoint,
			TokenURL:  asm.TokenEndpoint,
			AuthStyle: mcpOAuthTokenAuthStyle(asm, clientSecret),
		},
		RedirectURL: redirectBase + "/api/mcp/oauth/callback",
	}, nil
}

func (a *app) mcpOAuthCallbackClient(ctx context.Context, callbackState mcpOAuthCallbackState) (string, string) {
	if callbackState.serverID != 0 {
		list, _ := a.mcpStore.List(ctx, callbackState.userID)
		for _, s := range list {
			if s.ID == callbackState.serverID {
				return strings.TrimSpace(s.OAuthClientID), strings.TrimSpace(s.OAuthClientSecret)
			}
		}
	}
	return strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_ID")), strings.TrimSpace(os.Getenv("MCP_OAUTH_CLIENT_SECRET"))
}

func (a *app) persistMCPOAuthCallbackToken(
	ctx context.Context,
	callbackState mcpOAuthCallbackState,
	token *oauth2.Token,
) error {
	if callbackState.serverID == 0 {
		return nil
	}
	list, _ := a.mcpStore.List(ctx, callbackState.userID)
	for _, s := range list {
		if s.ID != callbackState.serverID {
			continue
		}
		s.OAuthAccessToken = token.AccessToken
		s.OAuthRefreshToken = token.RefreshToken
		s.OAuthExpiresAt = token.Expiry
		if _, err := a.mcpStore.Upsert(ctx, s.UserID, s); err != nil {
			return err
		}
		go a.reregisterMCPServerWithToken(s)
		break
	}
	return nil
}

func (a *app) reregisterMCPServerWithToken(server persistence.MCPServer) {
	ctx := context.Background()
	a.mcpManager.RemoveOne(server.Name, a.baseToolRegistry)
	if err := a.mcpManager.RegisterOne(ctx, a.baseToolRegistry, convertToConfig(server)); err != nil {
		fmt.Printf("mcp oauth re-register failed for %s: %v\n", server.Name, err)
	}
	a.refreshToolDiscoveryIndex()
}

func writeMCPOAuthCallbackSuccess(w http.ResponseWriter, token *oauth2.Token, targetURL string) {
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

// Discovery Helpers

type authServerMeta struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string
}

func (m *authServerMeta) UnmarshalJSON(data []byte) error {
	type authServerMetaAlias authServerMeta
	var decoded authServerMetaAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if methodsJSON, ok := raw[authServerTokenAuthMethodsKey]; ok {
		if err := json.Unmarshal(methodsJSON, &decoded.TokenEndpointAuthMethodsSupported); err != nil {
			return err
		}
	}
	*m = authServerMeta(decoded)
	return nil
}

func mcpOAuthTokenAuthStyle(asm *authServerMeta, clientSecret string) oauth2.AuthStyle {
	if asm == nil || len(asm.TokenEndpointAuthMethodsSupported) == 0 {
		return oauth2.AuthStyleAutoDetect
	}
	supports := func(method string) bool {
		for _, supported := range asm.TokenEndpointAuthMethodsSupported {
			if strings.EqualFold(strings.TrimSpace(supported), method) {
				return true
			}
		}
		return false
	}
	if clientSecret != "" {
		if supports(authMethodClientSecretPost) {
			return oauth2.AuthStyleInParams
		}
		if supports(authMethodClientSecretBasic) {
			return oauth2.AuthStyleInHeader
		}
		return oauth2.AuthStyleAutoDetect
	}
	if supports(authMethodNone) || supports(authMethodClientSecretPost) {
		return oauth2.AuthStyleInParams
	}
	return oauth2.AuthStyleAutoDetect
}

func mcpOAuthResourceParam(prm *oauthex.ProtectedResourceMetadata, fallback string) string {
	if prm != nil && strings.TrimSpace(prm.Resource) != "" {
		return strings.TrimSpace(prm.Resource)
	}
	return fallback
}

func mcpOAuthRegistrationGrantTypes(asm *authServerMeta) []string {
	grantTypes := []string{"authorization_code"}
	if asm == nil {
		return grantTypes
	}
	for _, grantType := range asm.GrantTypesSupported {
		if strings.EqualFold(strings.TrimSpace(grantType), "refresh_token") {
			return append(grantTypes, "refresh_token")
		}
	}
	return grantTypes
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
func (a *app) registerOAuthClient(ctx context.Context, registrationEndpoint, clientName, redirectURI string, scopes []string, grantTypes []string) (clientID, clientSecret string, err error) {
	body := map[string]any{
		"client_name":                        clientName,
		"client_uri":                         "https://github.com/intelligencedev/manifold",
		"grant_types":                        grantTypes,
		"response_types":                     []string{"code"},
		"redirect_uris":                      []string{redirectURI},
		clientRegistrationTokenAuthMethodKey: authMethodNone,
		"application_type":                   "native",
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

	refreshThreshold := time.Now().Add(5 * time.Minute)
	if srv.OAuthExpiresAt.After(refreshThreshold) {
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
