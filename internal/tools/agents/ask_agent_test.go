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

func TestAskAgent_ForwardsProjectAndSpecialist(t *testing.T) {
	var (
		gotBody  map[string]any
		gotQuery url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"{\"ok\":true,\"exit_code\":0,\"stdout\":\"hi\",\"stderr\":\"\"}"}`))
	}))
	defer srv.Close()

	tool := NewAskAgentTool(srv.Client(), srv.URL, 0)
	args := map[string]any{
		"prompt":     "hi",
		"to":         "spec",
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
	if gotQuery.Get("specialist") != "spec" {
		t.Fatalf("expected specialist query param, got %q", gotQuery.Get("specialist"))
	}
	if gotQuery.Get("stream") != "0" {
		t.Fatalf("expected stream=0, got %q", gotQuery.Get("stream"))
	}
	if pid, ok := gotBody["project_id"].(string); !ok || pid != "proj-123" {
		t.Fatalf("expected project_id proj-123, got %#v", gotBody["project_id"])
	}
	response := resp["response"].(map[string]any)
	inner := response["result"].(map[string]any)
	if ec, _ := inner["exit_code"].(float64); ec != 0 {
		t.Fatalf("expected decoded inner payload, got %#v", inner)
	}
}

func TestAskAgent_InheritsFromContext(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	tool := NewAskAgentTool(srv.Client(), srv.URL, 0)

	// Simulate context with session_id and project_id set by the HTTP handler
	ctx := context.Background()
	ctx = sandbox.WithSessionID(ctx, "ctx-session-456")
	ctx = sandbox.WithProjectID(ctx, "ctx-proj-789")

	// LLM only provides prompt, no explicit session/project
	args := map[string]any{
		"prompt": "do something",
	}
	raw, _ := json.Marshal(args)
	_, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	// Verify the context values were inherited
	if sid, ok := gotBody["session_id"].(string); !ok || sid != "ctx-session-456" {
		t.Fatalf("expected session_id from context, got %#v", gotBody["session_id"])
	}
	if pid, ok := gotBody["project_id"].(string); !ok || pid != "ctx-proj-789" {
		t.Fatalf("expected project_id from context, got %#v", gotBody["project_id"])
	}
}

func TestAskAgent_ExplicitOverridesContext(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	tool := NewAskAgentTool(srv.Client(), srv.URL, 0)

	// Context has default values
	ctx := context.Background()
	ctx = sandbox.WithSessionID(ctx, "ctx-session")
	ctx = sandbox.WithProjectID(ctx, "ctx-proj")

	// LLM explicitly provides different values (should override context)
	args := map[string]any{
		"prompt":     "do something",
		"session_id": "explicit-session",
		"project_id": "explicit-proj",
	}
	raw, _ := json.Marshal(args)
	_, err := tool.Call(ctx, raw)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}

	// Verify explicit values were used, not context values
	sid := gotBody["session_id"].(string)
	// Note: session_id gets mapped through UUID logic, but we can check it's not the context value
	if sid == "ctx-session" {
		t.Fatalf("explicit session_id should override context, got context value")
	}
	if pid, ok := gotBody["project_id"].(string); !ok || pid != "explicit-proj" {
		t.Fatalf("expected explicit project_id, got %#v", gotBody["project_id"])
	}
}
