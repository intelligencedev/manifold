package apidocs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSpecJSONIncludesCoreEndpoints(t *testing.T) {
	t.Parallel()

	data, err := GenerateSpecJSON(Options{
		ServerURL: "https://api.example.com",
	})
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "3.1.0", doc["openapi"])

	servers, ok := doc["servers"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, servers)
	server, ok := servers[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com", server["url"])

	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, paths, "/agent/run")
	require.Contains(t, paths, "/api/projects")
	require.Contains(t, paths, "/api/v1/playground/prompts")

	agentRunPath, ok := paths["/agent/run"].(map[string]any)
	require.True(t, ok)
	postOp, ok := agentRunPath["post"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Run orchestrator agent", postOp["summary"])
}

func TestGenerateSpecJSONAddsCookieAuthWhenEnabled(t *testing.T) {
	t.Parallel()

	data, err := GenerateSpecJSON(Options{
		AuthEnabled:    true,
		AuthCookieName: "manifold_session",
	})
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))

	components, ok := doc["components"].(map[string]any)
	require.True(t, ok)
	securitySchemes, ok := components["securitySchemes"].(map[string]any)
	require.True(t, ok)
	sessionCookie, ok := securitySchemes["sessionCookie"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "manifold_session", sessionCookie["name"])

	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok)

	projectsPath, ok := paths["/api/projects"].(map[string]any)
	require.True(t, ok)
	projectsGet, ok := projectsPath["get"].(map[string]any)
	require.True(t, ok)
	_, hasSecurity := projectsGet["security"]
	assert.True(t, hasSecurity, "authenticated endpoint should include security requirements")

	healthPath, ok := paths["/healthz"].(map[string]any)
	require.True(t, ok)
	healthGet, ok := healthPath["get"].(map[string]any)
	require.True(t, ok)
	_, hasSecurity = healthGet["security"]
	assert.False(t, hasSecurity, "public endpoint should not include security requirements")
}
