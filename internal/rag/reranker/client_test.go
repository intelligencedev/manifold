package reranker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
	"manifold/internal/rag/retrieve"
)

func TestClientRerankUsesEndpointOrderAndHeaders(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth string
	var gotBody requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(responseBody{Results: []responseResult{
			{Index: 1, RelevanceScore: floatPtr(0.91)},
			{Index: 0, RelevanceScore: floatPtr(0.12)},
		}})
	}))
	defer server.Close()

	client := NewClient(config.RerankingConfig{
		BaseURL:     server.URL,
		Model:       "reranker-model",
		Instruction: "Classify whether the document matches the query topic",
		APIKey:      "secret",
		APIHeader:   "Authorization",
		Path:        "/custom-rerank",
	})
	items := []retrieve.RetrievedItem{
		{ID: "a", Text: "first"},
		{ID: "b", Snippet: "second"},
		{ID: "c", Text: "third"},
	}

	out, err := client.Rerank(t.Context(), "query text", items)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if gotPath != "/custom-rerank" {
		t.Fatalf("unexpected request path %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	wantQuery := "Instruct: Classify whether the document matches the query topic\nQuery: query text"
	if gotBody.Model != "reranker-model" || gotBody.Query != wantQuery || gotBody.TopN != 3 {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
	if len(gotBody.Documents) != 3 || gotBody.Documents[1] != "second" {
		t.Fatalf("unexpected documents: %+v", gotBody.Documents)
	}
	if len(out) != 3 {
		t.Fatalf("expected all items to be preserved, got %d", len(out))
	}
	if out[0].ID != "b" || out[0].Score != 0.91 {
		t.Fatalf("unexpected first reranked item: %+v", out[0])
	}
	if out[1].ID != "a" || out[1].Score != 0.12 {
		t.Fatalf("unexpected second reranked item: %+v", out[1])
	}
	if out[2].ID != "c" {
		t.Fatalf("expected missing endpoint item appended, got %+v", out[2])
	}
	if got, ok := out[0].Explanation["rerank_score"].(float64); !ok || got != 0.91 {
		t.Fatalf("missing rerank explanation: %+v", out[0].Explanation)
	}
}

func TestClientRerankLeavesQueryRawWithoutInstruction(t *testing.T) {
	t.Parallel()

	var gotBody requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(responseBody{Results: []responseResult{
			{Index: 0, Score: floatPtr(0.5)},
			{Index: 1, Score: floatPtr(0.4)},
		}})
	}))
	defer server.Close()

	client := NewClient(config.RerankingConfig{BaseURL: server.URL})
	_, err := client.Rerank(t.Context(), " query text ", []retrieve.RetrievedItem{
		{ID: "a", Text: "first"},
		{ID: "b", Text: "second"},
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if gotBody.Query != "query text" {
		t.Fatalf("expected raw trimmed query, got %q", gotBody.Query)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}
