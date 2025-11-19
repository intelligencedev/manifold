package ingest

import (
	"context"
	"testing"
)

func TestComputeHash_DeterministicAndUriSensitive(t *testing.T) {
	text := "Hello, world!\n\nThis is a test."
	h1 := ComputeHash(text, "web", "https://example.com/a")
	h2 := ComputeHash(text, "web", "https://example.com/a")
	if h1 != h2 {
		t.Fatalf("expected same hash, got %s vs %s", h1, h2)
	}
	h3 := ComputeHash(text+" ", "web", "https://example.com/a")
	if h1 == h3 {
		t.Fatalf("expected different hash when text changes")
	}
	h4 := ComputeHash(text, "web", "https://example.com/b")
	if h1 == h4 {
		t.Fatalf("expected different hash when URL changes")
	}
}

func TestPreprocess_NormalizesAndDefaultsLang(t *testing.T) {
	in := IngestRequest{Text: "Hello\r\n\r\nworld\t!", Source: "x", URL: "u"}
	pre, err := Preprocess(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("preprocess error: %v", err)
	}
	if pre.Language != "english" {
		t.Fatalf("expected english, got %s", pre.Language)
	}
	if pre.Text != "Hello\n\nworld !" {
		t.Fatalf("unexpected normalization: %q", pre.Text)
	}
}
