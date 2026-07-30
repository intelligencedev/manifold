package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestNormalizeMaxBytes(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want int64
	}{
		{"omitted defaults to 8MB", 0, defaultFetchMaxBytes},
		{"negative defaults to 8MB", -1, defaultFetchMaxBytes},
		{"below floor clamps up to 1MB", 500000, minFetchMaxBytes},
		{"at floor stays 1MB", minFetchMaxBytes, minFetchMaxBytes},
		{"in range passes through", 5000000, 5000000},
		{"above ceiling clamps to 16MB", 99000000, maxFetchMaxBytes},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeMaxBytes(c.in); got != c.want {
				t.Fatalf("normalizeMaxBytes(%d) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestFetchMarkdown_TruncatesOversizedBody(t *testing.T) {
	// Article content near the top, then padding that pushes the raw body well
	// past the configured MaxBytes.
	head := "<html><head><title>Big</title></head><body>" +
		"<h1>Headline</h1><p>The important article body lives up here.</p>"
	padding := strings.Repeat("<p>filler filler filler filler</p>", 4000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, head+padding+"</body></html>")
	}))
	defer srv.Close()

	f := NewFetcher(WithMaxBytes(2000), WithTimeout(2*time.Second))
	res, err := f.FetchMarkdown(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("oversized body should truncate, not error: %v", err)
	}
	if res.Markdown == "" {
		t.Fatalf("expected markdown extracted from the truncated body")
	}
	if !strings.Contains(res.Markdown, "Headline") {
		t.Fatalf("expected leading content to survive truncation, got: %q", res.Markdown)
	}
}
