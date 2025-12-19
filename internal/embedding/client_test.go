package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"manifold/internal/config"
)

func TestEmbedText_HeadersMapAuthorization(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Token abc" {
			t.Fatalf("expected Authorization header Token abc, got %q", got)
		}
		// return minimal valid response
		resp := map[string]interface{}{"data": []map[string]interface{}{{"embedding": []float32{0.1}}}}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer ts.Close()

	cfg := config.EmbeddingConfig{BaseURL: ts.URL, Path: "/", Model: "m", Headers: map[string]string{"Authorization": "Token abc"}}
	_, err := EmbedText(context.Background(), cfg, []string{"x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmbedText_LegacyAuthorization(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("expected Authorization header Bearer secret, got %q", got)
		}
		resp := map[string]interface{}{"data": []map[string]interface{}{{"embedding": []float32{0.1}}}}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer ts.Close()

	cfg := config.EmbeddingConfig{BaseURL: ts.URL, Path: "/", Model: "m", APIHeader: "Authorization", APIKey: "secret"}
	_, err := EmbedText(context.Background(), cfg, []string{"x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmbedText_MixedHeadersPrecedence(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "abc" {
			t.Fatalf("expected x-api-key header abc, got %q", got)
		}
		// Authorization should be set from legacy when not present in headers map
		if got := r.Header.Get("Authorization"); got != "Bearer s" {
			t.Fatalf("expected Authorization header Bearer s, got %q", got)
		}
		resp := map[string]interface{}{"data": []map[string]interface{}{{"embedding": []float32{0.1}}}}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer ts.Close()

	cfg := config.EmbeddingConfig{BaseURL: ts.URL, Path: "/", Model: "m", APIHeader: "Authorization", APIKey: "s", Headers: map[string]string{"x-api-key": "abc"}}
	_, err := EmbedText(context.Background(), cfg, []string{"x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
