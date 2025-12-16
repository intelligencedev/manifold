package agentd

// NOTE: These tests exercise MCP server CRUD + OAuth flows including dynamic client
// registration, PKCE, resource indicator, and token persistence. They avoid global
// auth by leaving cfg.Auth.Enabled=false so requireUserID returns systemUserID.
//
// WARNING: The repository build may currently fail due to missing whisper.cpp
// CGO headers. Running only these tests still compiles the agentd package which
// imports code referring to whisper; ensure the whisper dependency is set up or
// run with environment that provides the header. The test logic itself does not
// depend on whisper.

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
	a := &app{cfg: cfg, httpClient: httpClient, mcpStore: store, toolRegistry: stubRegistry{}}
	return a
}

// parseJSON helper.
func parseJSON[T any](t *testing.T, body io.Reader) T {
	t.Helper()
	var v T
	dec := json.NewDecoder(body)
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	return v
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
	if !strings.Contains(redirect, "/api/mcp/oauth/callback") {
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
