package specialists

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", " ", "a", "b") != "a" {
		t.Fatalf("unexpected firstNonEmpty")
	}
}

func TestNamesSorted(t *testing.T) {
	r := &Registry{agents: map[string]*Agent{"z": {}, "a": {}, "m": {}}}
	n := r.Names()
	if len(n) != 3 || n[0] != "a" || n[1] != "m" || n[2] != "z" {
		t.Fatalf("unexpected order: %#v", n)
	}
}

// fakeRoundTripper records headers passed in requests and returns a fixed response.
type fakeRoundTripper struct{ last http.Header }

func (f *fakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	f.last = r.Header.Clone()
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}
	return resp, nil
}

func TestHeaderTransport(t *testing.T) {
	base := &fakeRoundTripper{}
	tx := &headerTransport{base: base, headers: map[string]string{"X-Test": "v"}}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://example/", nil)
	_, err := tx.RoundTrip(req)
	if err != nil {
		t.Fatalf("roundtrip failed: %v", err)
	}
	if base.last.Get("X-Test") != "v" {
		t.Fatalf("header missing: %#v", base.last)
	}
}
