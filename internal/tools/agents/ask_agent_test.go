package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
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
