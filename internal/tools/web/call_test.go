package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebTool_Call_JSONResults(t *testing.T) {
	// Start a server that returns JSON results for format=json
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("format") == "json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"results":[{"title":"One","url":"https://example.com/1"},{"title":"Two","url":"https://example.com/2"}]}`))
			return
		}
		w.WriteHeader(500)
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	tool := NewTool(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := map[string]any{"query": "x", "max_results": 2}
	raw, _ := json.Marshal(args)
	res, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %#v", res)
	}
	if okv, _ := m["ok"].(bool); !okv {
		t.Fatalf("expected ok true, got %#v", m)
	}
	// results may be []web.SearchResult (concrete) or []any depending on path
	switch r := m["results"].(type) {
	case []any:
		if len(r) != 2 {
			t.Fatalf("expected 2 results, got %d", len(r))
		}
	case []SearchResult:
		if len(r) != 2 {
			t.Fatalf("expected 2 results, got %d", len(r))
		}
	default:
		t.Fatalf("unexpected results type: %T", r)
	}
}

func TestWebTool_Call_HTMLFallback(t *testing.T) {
	// Server returns 500 for JSON and HTML with links otherwise
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("format") == "json" {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><a href="https://a.example/1">Link1</a><a href="https://a.example/2">Link2</a></body></html>`))
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	tool := NewTool(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := map[string]any{"query": "x", "max_results": 5}
	raw, _ := json.Marshal(args)
	res, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %#v", res)
	}
	if okv, _ := m["ok"].(bool); !okv {
		t.Fatalf("expected ok true, got %#v", m)
	}
	switch r := m["results"].(type) {
	case []any:
		if len(r) != 2 {
			t.Fatalf("expected 2 results, got %d", len(r))
		}
	case []SearchResult:
		if len(r) != 2 {
			t.Fatalf("expected 2 results, got %d", len(r))
		}
	default:
		t.Fatalf("unexpected results type: %T", r)
	}
}
