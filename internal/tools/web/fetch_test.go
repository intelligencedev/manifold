package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseContentTypeAndHelpers(t *testing.T) {
	ct, cs := parseContentType("text/html; charset=utf-8")
	if ct != "text/html" || cs != "utf-8" {
		t.Fatalf("parseContentType failed: %v %v", ct, cs)
	}
	if !isHTML("text/html") || !isHTML("application/xhtml+xml") {
		t.Fatalf("isHTML failed")
	}
	if !hasLeadingH1("# Title\ncontent") {
		t.Fatalf("hasLeadingH1 failed")
	}
	if fenced("a\n", "md") == "" {
		t.Fatalf("fenced returned empty")
	}
}

func TestToUTF8(t *testing.T) {
	// UTF-8 passes through
	b, err := toUTF8([]byte("hello"), "utf-8")
	if err != nil || string(b) != "hello" {
		t.Fatalf("toUTF8 utf8 failed: %v", err)
	}
}

func TestFetchMarkdown_HTMLAndText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(200)
			_, _ = w.Write([]byte("<html><head><title>X</title></head><body><h1>Hi</h1><p>There</p></body></html>"))
			return
		}
		if r.URL.Path == "/text" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("plain text"))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	f := NewFetcher(WithMaxBytes(1024), WithTimeout(2*time.Second))
	ctx := context.Background()
	res, err := f.FetchMarkdown(ctx, srv.URL+"/html")
	if err != nil {
		t.Fatalf("fetch html failed: %v", err)
	}
	if res.Markdown == "" {
		t.Fatalf("expected markdown for html")
	}

	res2, err := f.FetchMarkdown(ctx, srv.URL+"/text")
	if err != nil {
		t.Fatalf("fetch text failed: %v", err)
	}
	if res2.Markdown == "" {
		t.Fatalf("expected markdown for text")
	}
}

func TestFetchMarkdown_NonText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("binarydata"))
	}))
	defer srv.Close()
	f := NewFetcher(WithMaxBytes(16))
	res, err := f.FetchMarkdown(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch binary failed: %v", err)
	}
	if res.Markdown == "" {
		t.Fatalf("expected stub for binary")
	}
}
func TestNewFetcherTransportLimits(t *testing.T) {
	f := NewFetcher()
	if f == nil || f.client == nil {
		t.Fatal("NewFetcher returned nil client")
	}
	tr, ok := f.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", f.client.Transport)
	}
	if tr.MaxConnsPerHost < 100 {
		t.Fatalf("MaxConnsPerHost too low: %d", tr.MaxConnsPerHost)
	}
	if tr.MaxIdleConnsPerHost < 50 {
		t.Fatalf("MaxIdleConnsPerHost too low: %d", tr.MaxIdleConnsPerHost)
	}
}
