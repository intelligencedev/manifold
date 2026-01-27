package agentd

// NOTE: These tests exercise MCP server CRUD + OAuth flows including dynamic client
// registration, PKCE, resource indicator, and token persistence. They avoid global
// auth by leaving cfg.Auth.Enabled=false so requireUserID returns systemUserID.
//
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	oauthex "github.com/modelcontextprotocol/go-sdk/oauthex"

	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/mcpclient"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/tools"
)

// stubRegistry is a minimal tools.Registry implementation for tests.
type stubRegistry struct{}

func (stubRegistry) Schemas() []llm.ToolSchema { return nil }
func (stubRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	return []byte(`{}`), nil
}
func (stubRegistry) Register(t tools.Tool)  {}
func (stubRegistry) Unregister(name string) {}

// roundTripFunc allows customizing HTTP responses.
type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

// newTestHTTPClient builds an http.Client with a custom transport.
func newTestHTTPClient(fn roundTripFunc) *http.Client { return &http.Client{Transport: fn} }

// buildTestApp constructs a minimal *app suitable for handler tests.
func buildTestApp(t *testing.T, httpClient *http.Client, store persist.MCPStore) *app {
	t.Helper()
	cfg := &config.Config{}
	cfg.Auth.RedirectURL = "http://localhost:32180/auth/callback" // simulate configured auth redirect path
	// Auth.Enabled=false so requireUserID returns system user
	a := &app{
		cfg:          cfg,
		httpClient:   httpClient,
		mcpStore:     store,
		toolRegistry: stubRegistry{},
		mcpManager:   mcpclient.NewManager(),
	}
	return a
}

// TestMCPCreateAndList verifies basic create + list behaviour.
func TestMCPCreateAndList(t *testing.T) {
	store := databases.NewMCPStore(nil) // in-memory
	// Pre-init not required for mem store
	client := newTestHTTPClient(func(r *http.Request) *http.Response {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}
	})
	a := buildTestApp(t, client, store)

	// Create server
	createHandler := a.mcpServersHandler()
	body := strings.NewReader(`{"name":"test","url":"https://resource.example/data"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/servers", body)
	rec := httptest.NewRecorder()
	createHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	var saved persist.MCPServer
	json.Unmarshal(rec.Body.Bytes(), &saved)
	if saved.Name != "test" {
		t.Fatalf("wrong name %s", saved.Name)
	}

	// List servers should include the DB server
	listReq := httptest.NewRequest(http.MethodGet, "/api/mcp/servers", nil)
	listRec := httptest.NewRecorder()
	createHandler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d", listRec.Code)
	}
	var out []map[string]any
	json.Unmarshal(listRec.Body.Bytes(), &out)
	found := false
	for _, m := range out {
		if m["name"] == "test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created server not found in list response")
	}
}

// TestMCPOAuthStartDynamicRegistration exercises the start handler performing dynamic registration.
func TestMCPOAuthStartDynamicRegistration(t *testing.T) {
	store := databases.NewMCPStore(nil)
	// Insert server without client id
	srv, err := store.Upsert(context.Background(), 0, persist.MCPServer{Name: "dyn", URL: "https://resource.example/data"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Transport script: respond to resource metadata, auth metadata, registration endpoint.
	transport := func(r *http.Request) *http.Response {
		switch {
		case strings.Contains(r.URL.Path, "/.well-known/oauth-protected-resource"):
			meta := oauthex.ProtectedResourceMetadata{Resource: "https://resource.example", AuthorizationServers: []string{"https://auth.example"}, ScopesSupported: []string{"openid", "profile"}}
			b, _ := json.Marshal(meta)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}
		case strings.Contains(r.URL.Host, "auth.example") && strings.Contains(r.URL.Path, "/.well-known/oauth-authorization-server"):
			// Authorization server metadata including registration endpoint
			b := `{"issuer":"https://auth.example","authorization_endpoint":"https://auth.example/authorize","token_endpoint":"https://auth.example/token","registration_endpoint":"https://auth.example/register"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b))}
		case r.Method == http.MethodPost && r.URL.String() == "https://auth.example/register":
			b := `{"client_id":"cid123","client_secret":"sec123"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b))}
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}
		}
	}
	client := newTestHTTPClient(transport)
	a := buildTestApp(t, client, store)
	h := a.mcpOAuthStartHandler()

	body := strings.NewReader(fmt.Sprintf(`{"serverId":%d}`, srv.ID))
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/oauth/start", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	redirect := resp["redirectUrl"]
	if !strings.Contains(redirect, "client_id=cid123") {
		t.Fatalf("redirect URL missing client_id: %s", redirect)
	}
	if strings.Contains(redirect, "/auth/callback/api/mcp/oauth/callback") {
		t.Fatalf("redirect URL not trimmed: %s", redirect)
	}
	// Check that redirect_uri parameter contains our callback path (URL encoded)
	if !strings.Contains(redirect, url.QueryEscape("/api/mcp/oauth/callback")) {
		t.Fatalf("redirect URL missing callback path: %s", redirect)
	}
	// Verify store updated
	updated, ok, _ := store.GetByName(context.Background(), 0, "dyn")
	if !ok || updated.OAuthClientID != "cid123" {
		t.Fatalf("dynamic registration not persisted")
	}
}

// TestMCPOAuthCallbackTokenExchange simulates completing OAuth code flow.
func TestMCPOAuthCallbackTokenExchange(t *testing.T) {
	store := databases.NewMCPStore(nil)
	// Persist server with client id from prior registration
	srv, _ := store.Upsert(context.Background(), 0, persist.MCPServer{Name: "cb", URL: "https://resource.example/data", OAuthClientID: "cid123"})

	// Dynamic responses for metadata + token endpoint.
	transport := func(r *http.Request) *http.Response {
		switch {
		case strings.Contains(r.URL.Path, "/.well-known/oauth-protected-resource"):
			meta := oauthex.ProtectedResourceMetadata{Resource: "https://resource.example", AuthorizationServers: []string{"https://auth.example"}, ScopesSupported: []string{"openid"}}
			b, _ := json.Marshal(meta)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}
		case strings.Contains(r.URL.Host, "auth.example") && strings.Contains(r.URL.Path, "/.well-known/oauth-authorization-server"):
			b := `{"issuer":"https://auth.example","authorization_endpoint":"https://auth.example/authorize","token_endpoint":"https://auth.example/token"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b))}
		case r.Method == http.MethodPost && r.URL.String() == "https://auth.example/token":
			b := `{"access_token":"atk123","refresh_token":"rtk456","expires_in": 3600,"token_type":"Bearer"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b))}
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}
		}
	}
	client := newTestHTTPClient(transport)
	a := buildTestApp(t, client, store)

	// Manually set cookies as if from start handler.
	state := "state-xyz"
	// state cookie format: state|targetURL|userID|serverID
	stateVal := strings.Join([]string{state, srv.URL, "0", fmt.Sprintf("%d", srv.ID)}, "|")
	callbackURL := "http://localhost/api/mcp/oauth/callback?state=" + url.QueryEscape(state) + "&code=authcode123"
	req := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.AddCookie(&http.Cookie{Name: "mcp_oauth_state", Value: stateVal})
	req.AddCookie(&http.Cookie{Name: "mcp_oauth_pkce", Value: "verifier123"})
	rec := httptest.NewRecorder()
	a.mcpOAuthCallbackHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 callback, got %d", rec.Code)
	}
	// Validate persistence
	updated, ok, _ := store.GetByName(context.Background(), 0, "cb")
	if !ok || updated.OAuthAccessToken != "atk123" || updated.OAuthRefreshToken != "rtk456" {
		t.Fatalf("token fields not persisted: %+v", updated)
	}
	if time.Until(updated.OAuthExpiresAt) <= 0 {
		t.Fatalf("expiry not set")
	}
}

// TestMCPOAuthTokenRefresh tests the token refresh functionality when tokens are expired.
func TestMCPOAuthTokenRefresh(t *testing.T) {
	store := databases.NewMCPStore(nil)
	// Create server with expired token and refresh token
	expiredTime := time.Now().Add(-1 * time.Hour) // Expired 1 hour ago
	srv, _ := store.Upsert(context.Background(), 0, persist.MCPServer{
		Name:              "refresh-test",
		URL:               "https://resource.example/data",
		OAuthClientID:     "cid123",
		OAuthClientSecret: "sec123",
		OAuthAccessToken:  "old-token",
		OAuthRefreshToken: "refresh-token",
		OAuthExpiresAt:    expiredTime,
	})

	// Transport that handles metadata discovery and token refresh
	transport := func(r *http.Request) *http.Response {
		switch {
		case strings.Contains(r.URL.Path, "/.well-known/oauth-protected-resource"):
			meta := oauthex.ProtectedResourceMetadata{
				Resource:             "https://resource.example",
				AuthorizationServers: []string{"https://auth.example"},
				ScopesSupported:      []string{"openid"},
			}
			b, _ := json.Marshal(meta)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}
		case strings.Contains(r.URL.Host, "auth.example") && strings.Contains(r.URL.Path, "/.well-known/oauth-authorization-server"):
			b := `{"issuer":"https://auth.example","authorization_endpoint":"https://auth.example/authorize","token_endpoint":"https://auth.example/token"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b))}
		case r.Method == http.MethodPost && r.URL.String() == "https://auth.example/token":
			// Token refresh endpoint - return new tokens
			b := `{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600,"token_type":"Bearer"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b))}
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}
		}
	}
	client := newTestHTTPClient(transport)
	a := buildTestApp(t, client, store)

	// Load the server and refresh
	refreshed, needsAuth, err := a.refreshOAuthTokenIfNeeded(context.Background(), srv)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if needsAuth {
		t.Fatalf("should not need re-auth when refresh token is available")
	}
	if refreshed.OAuthAccessToken != "new-access-token" {
		t.Fatalf("expected new access token, got %s", refreshed.OAuthAccessToken)
	}
	if refreshed.OAuthRefreshToken != "new-refresh-token" {
		t.Fatalf("expected new refresh token, got %s", refreshed.OAuthRefreshToken)
	}
	if refreshed.OAuthExpiresAt.Before(time.Now()) {
		t.Fatalf("new expiry should be in the future")
	}

	// Verify persistence
	stored, ok, _ := store.GetByName(context.Background(), 0, "refresh-test")
	if !ok {
		t.Fatal("server not found in store")
	}
	if stored.OAuthAccessToken != "new-access-token" {
		t.Fatalf("token not persisted, got %s", stored.OAuthAccessToken)
	}
}

// TestMCPOAuthTokenRefreshNoRefreshToken tests that re-auth is required when no refresh token exists.
func TestMCPOAuthTokenRefreshNoRefreshToken(t *testing.T) {
	store := databases.NewMCPStore(nil)
	// Create server with expired token but NO refresh token
	expiredTime := time.Now().Add(-1 * time.Hour)
	srv, _ := store.Upsert(context.Background(), 0, persist.MCPServer{
		Name:             "no-refresh-test",
		URL:              "https://resource.example/data",
		OAuthClientID:    "cid123",
		OAuthAccessToken: "old-token",
		OAuthExpiresAt:   expiredTime,
		// No OAuthRefreshToken
	})

	client := newTestHTTPClient(func(r *http.Request) *http.Response {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}
	})
	a := buildTestApp(t, client, store)

	refreshed, needsAuth, _ := a.refreshOAuthTokenIfNeeded(context.Background(), srv)
	if !needsAuth {
		t.Fatal("should require re-auth when no refresh token available")
	}
	if refreshed.OAuthAccessToken != "" {
		t.Fatalf("access token should be cleared, got %s", refreshed.OAuthAccessToken)
	}
}

// TestMCPOAuthTokenValidNotExpired tests that valid tokens are not refreshed.
func TestMCPOAuthTokenValidNotExpired(t *testing.T) {
	store := databases.NewMCPStore(nil)
	// Create server with valid token (expires in 1 hour)
	validExpiry := time.Now().Add(1 * time.Hour)
	srv, _ := store.Upsert(context.Background(), 0, persist.MCPServer{
		Name:              "valid-test",
		URL:               "https://resource.example/data",
		OAuthClientID:     "cid123",
		OAuthAccessToken:  "valid-token",
		OAuthRefreshToken: "refresh-token",
		OAuthExpiresAt:    validExpiry,
	})

	// Transport should NOT be called for valid tokens
	callCount := 0
	client := newTestHTTPClient(func(r *http.Request) *http.Response {
		callCount++
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("should not be called"))}
	})
	a := buildTestApp(t, client, store)

	refreshed, needsAuth, err := a.refreshOAuthTokenIfNeeded(context.Background(), srv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if needsAuth {
		t.Fatal("should not need re-auth for valid token")
	}
	if callCount > 0 {
		t.Fatal("HTTP client should not have been called for valid token")
	}
	if refreshed.OAuthAccessToken != "valid-token" {
		t.Fatalf("token should be unchanged, got %s", refreshed.OAuthAccessToken)
	}
}

// TestMCPOAuthTokenRefreshFails tests behavior when refresh request fails.
func TestMCPOAuthTokenRefreshFails(t *testing.T) {
	store := databases.NewMCPStore(nil)
	expiredTime := time.Now().Add(-1 * time.Hour)
	srv, _ := store.Upsert(context.Background(), 0, persist.MCPServer{
		Name:              "refresh-fail-test",
		URL:               "https://resource.example/data",
		OAuthClientID:     "cid123",
		OAuthAccessToken:  "old-token",
		OAuthRefreshToken: "refresh-token",
		OAuthExpiresAt:    expiredTime,
	})

	// Transport that returns error on token refresh
	transport := func(r *http.Request) *http.Response {
		switch {
		case strings.Contains(r.URL.Path, "/.well-known/oauth-protected-resource"):
			meta := oauthex.ProtectedResourceMetadata{
				Resource:             "https://resource.example",
				AuthorizationServers: []string{"https://auth.example"},
				ScopesSupported:      []string{"openid"},
			}
			b, _ := json.Marshal(meta)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}
		case strings.Contains(r.URL.Host, "auth.example") && strings.Contains(r.URL.Path, "/.well-known/oauth-authorization-server"):
			b := `{"issuer":"https://auth.example","authorization_endpoint":"https://auth.example/authorize","token_endpoint":"https://auth.example/token"}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b))}
		case r.Method == http.MethodPost && r.URL.String() == "https://auth.example/token":
			// Simulate refresh token revoked/invalid
			return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`))}
		default:
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}
		}
	}
	client := newTestHTTPClient(transport)
	a := buildTestApp(t, client, store)

	refreshed, needsAuth, err := a.refreshOAuthTokenIfNeeded(context.Background(), srv)
	if err == nil {
		t.Fatal("expected error when refresh fails")
	}
	if !needsAuth {
		t.Fatal("should require re-auth when refresh fails")
	}
	if refreshed.OAuthAccessToken != "" {
		t.Fatal("tokens should be cleared on refresh failure")
	}
	if refreshed.OAuthRefreshToken != "" {
		t.Fatal("refresh token should be cleared on refresh failure")
	}

	// Verify tokens are cleared in store
	stored, ok, _ := store.GetByName(context.Background(), 0, "refresh-fail-test")
	if !ok {
		t.Fatal("server not found")
	}
	if stored.OAuthAccessToken != "" || stored.OAuthRefreshToken != "" {
		t.Fatal("tokens should be cleared in store after refresh failure")
	}
}
