package agentd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrepareChatTransportHandlesPromptPreflight(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodOptions, "/api/prompt", nil)
	rr := httptest.NewRecorder()

	_, ok := prepareChatTransport(rr, req, chatTransportOptions{EnablePromptCORS: true})
	if ok {
		t.Fatal("expected preflight to be handled without decoding a request")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard origin, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "POST, OPTIONS" {
		t.Fatalf("expected prompt methods header, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Accept" {
		t.Fatalf("expected prompt allow headers, got %q", got)
	}
}

func TestPrepareChatTransportDecodesAndNormalizesPostBody(t *testing.T) {
	t.Parallel()

	body := bytes.NewBufferString(`{"prompt":"hello","session_id":"   "}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run", body)
	rr := httptest.NewRecorder()

	decoded, ok := prepareChatTransport(rr, req, chatTransportOptions{})
	if !ok {
		t.Fatalf("expected POST body to decode: %d %s", rr.Code, rr.Body.String())
	}
	if decoded.Prompt != "hello" {
		t.Fatalf("expected prompt to decode, got %q", decoded.Prompt)
	}
	if decoded.SessionID != "default" {
		t.Fatalf("expected normalized default session, got %q", decoded.SessionID)
	}
}
