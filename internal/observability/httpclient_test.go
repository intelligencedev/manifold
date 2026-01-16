package observability

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWithHeaders_InsertsHeaders(t *testing.T) {
	base := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("X-Test"); got != "v" {
			t.Fatalf("header not injected: got %q", got)
		}
		// Also ensure we don't override already-set headers.
		if got := req.Header.Get("X-Existing"); got != "keep" {
			t.Fatalf("existing header overwritten: got %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}

	c := WithHeaders(base, map[string]string{"X-Test": "v", "X-Existing": "override"})
	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Existing", "keep")
	if _, err := c.Do(req); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestNewHTTPClient_NotNil(t *testing.T) {
	c := NewHTTPClient(nil)
	if c == nil {
		t.Fatalf("expected non-nil client")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
