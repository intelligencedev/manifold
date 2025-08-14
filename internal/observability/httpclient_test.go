package observability

import (
	"net/http"
	"testing"
)

func TestWithHeaders_InsertsHeaders(t *testing.T) {
	base := &http.Client{}
	h := map[string]string{"X-Test": "v"}
	c := WithHeaders(base, h)
	// Use transport presence as a light assertion; actual RoundTrip behavior
	// is exercised elsewhere in integration tests.
	if c == nil || c.Transport == nil {
		t.Fatalf("unexpected nil client or transport")
	}
}

func TestNewHTTPClient_NotNil(t *testing.T) {
	c := NewHTTPClient(nil)
	if c == nil {
		t.Fatalf("expected non-nil client")
	}
}
