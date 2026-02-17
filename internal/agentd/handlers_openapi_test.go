package agentd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"manifold/internal/config"
)

func TestOpenAPISpecHandlerReturnsJSON(t *testing.T) {
	t.Parallel()

	a := &app{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/openapi.json", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "api.example.com")
	rr := httptest.NewRecorder()

	a.openapiSpecHandler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")

	var doc map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &doc))
	servers, ok := doc["servers"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, servers)
	server, ok := servers[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com", server["url"])
}

func TestOpenAPISpecHandlerRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	a := &app{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/openapi.json", nil)
	rr := httptest.NewRecorder()

	a.openapiSpecHandler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestOpenAPIDocsHandlerReturnsSwaggerPage(t *testing.T) {
	t.Parallel()

	a := &app{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/api-docs", nil)
	rr := httptest.NewRecorder()

	a.openapiDocsHandler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
	body := rr.Body.String()
	assert.True(t, strings.Contains(body, "SwaggerUIBundle"))
	assert.True(t, strings.Contains(body, "/openapi.json"))
}
