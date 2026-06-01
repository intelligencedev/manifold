package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/agent/memory/magma"
	"manifold/internal/config"
	"manifold/internal/persistence/databases"
	"manifold/internal/rag/embedder"
	ragservice "manifold/internal/rag/service"
)

func TestHandleMemoryObservabilityExplainDefaultsTenantWhenAuthDisabled(t *testing.T) {
	t.Parallel()

	mgr := databases.Manager{
		Search: databases.NewMemorySearch(),
		Vector: databases.NewMemoryVector(),
		Graph:  databases.NewMemoryGraph(),
	}
	magmaSvc := magma.NewService(mgr.Graph, mgr.Vector, embedder.NewDeterministic(32, true, 0))
	ragSvc := ragservice.New(mgr, ragservice.WithMagmaService(magmaSvc))
	t.Cleanup(ragSvc.Close)

	resp, err := magmaSvc.Ingest(context.Background(), magma.IngestRequest{
		ID:     "test-jax",
		Tenant: "user:0",
		Text:   "Jax prefers concise retrieval explanations.",
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if resp.EventID == "" {
		t.Fatalf("expected event id")
	}

	a := &app{
		cfg: &config.Config{
			Magma: config.MagmaConfig{
				Retrieval: config.MagmaRetrievalConfig{
					DefaultHops:          1,
					DefaultMaxNodes:      4,
					ContextFormat:        "structured",
					IntentClassification: "rules",
				},
			},
		},
		ragService: ragSvc,
	}

	for _, path := range []string{
		"/api/observability/memory/retrieval/explain?q=jax",
		"/api/observability/memory/retrieval/explain?q=jax&tenant=wrong",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		a.handleMemoryObservabilityExplain(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d body=%s", path, rec.Code, rec.Body.String())
		}
		var got memoryObservabilityExplainResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: unmarshal response: %v", path, err)
		}
		if got.AnchorCount == 0 || len(got.Events) == 0 {
			t.Fatalf("%s: expected user:0 retrieval anchors and events, got %#v", path, got)
		}
		if got.Events[0].Tenant != "user:0" {
			t.Fatalf("%s: expected user:0 event, got %#v", path, got.Events[0])
		}
	}
}
