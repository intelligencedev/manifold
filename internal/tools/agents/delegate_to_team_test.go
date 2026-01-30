package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"manifold/internal/sandbox"
)

func TestDelegateToTeam_ForwardsTeamAndProject(t *testing.T) {
	var (
		gotBody  map[string]any
		gotQuery url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"{\"ok\":true}"}`))
	}))
	defer srv.Close()

	tool := NewDelegateToTeamTool(srv.Client(), srv.URL, 0)
	args := map[string]any{
		"team":       "alpha",
		"prompt":     "do work",
		"project_id": "proj-123",
		"session_id": "abc",
	}
	raw, _ := json.Marshal(args)
	out, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}
	resp, ok := out.(map[string]any)
	if !ok || resp["ok"] != true {
		t.Fatalf("unexpected response: %#v", out)
	}
	if gotQuery.Get("group") != "alpha" {
		t.Fatalf("expected group query param, got %q", gotQuery.Get("group"))
	}
	if gotQuery.Get("stream") != "0" {
		t.Fatalf("expected stream=0, got %q", gotQuery.Get("stream"))
	}
	if pid, ok := gotBody["project_id"].(string); !ok || pid != "proj-123" {
		t.Fatalf("expected project_id proj-123, got %#v", gotBody["project_id"])
	}
}

func TestDelegateToTeam_InheritsFromContext(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	tool := NewDelegateToTeamTool(srv.Client(), srv.URL, 0)

	ctx := context.Background()
	ctx = sandbox.WithSessionID(ctx, "ctx-session-456")
	ctx = sandbox.WithProjectID(ctx, "ctx-proj-789")

	args := map[string]any{
		"team":   "beta",
		"prompt": "do something",
	}
	raw, _ := json.Marshal(args)
	_, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	if sid, ok := gotBody["session_id"].(string); !ok || sid != "ctx-session-456" {
		t.Fatalf("expected session_id from context, got %#v", gotBody["session_id"])
	}
	if pid, ok := gotBody["project_id"].(string); !ok || pid != "ctx-proj-789" {
		t.Fatalf("expected project_id from context, got %#v", gotBody["project_id"])
	}
}

func TestDelegateToTeam_ForwardsAuthCookie(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	tool := NewDelegateToTeamTool(srv.Client(), srv.URL, 0)

	// Context with auth cookie set
	ctx := sandbox.WithAuthCookie(context.Background(), "sio_session=abc123")

	args := map[string]any{
		"team":   "gamma",
		"prompt": "test auth",
	}
	raw, _ := json.Marshal(args)
	_, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	if gotCookie != "sio_session=abc123" {
		t.Fatalf("expected Cookie header 'sio_session=abc123', got %q", gotCookie)
	}
}

func TestDelegateToTeam_NoCookieWhenNotInContext(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	tool := NewDelegateToTeamTool(srv.Client(), srv.URL, 0)

	// Context without auth cookie
	args := map[string]any{
		"team":   "delta",
		"prompt": "test no auth",
	}
	raw, _ := json.Marshal(args)
	_, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	if gotCookie != "" {
		t.Fatalf("expected no Cookie header, got %q", gotCookie)
	}
}
