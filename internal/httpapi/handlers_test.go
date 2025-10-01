package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/playground"
	"manifold/internal/playground/provider"
	"manifold/internal/playground/registry"

	"github.com/stretchr/testify/require"
)

func TestCreatePromptEndpoint(t *testing.T) {
	svc := playground.NewMockService(provider.NewMockProvider("mock"))
	srv := NewServer(svc)

	body, err := json.Marshal(registry.Prompt{ID: "p1", Name: "Prompt"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/prompts", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
}
