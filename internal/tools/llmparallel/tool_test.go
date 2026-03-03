package llmparallel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParallelCompletionsWithSynthesis(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		prompt, _ := body["prompt"].(string)
		if strings.Contains(prompt, "expert response aggregator") {
			_, _ = w.Write([]byte(`{"choices":[{"text":"Merged best answer"}]}`))
			atomic.AddInt32(&calls, 1)
			return
		}
		idx := atomic.AddInt32(&calls, 1)
		resp := "candidate"
		switch idx {
		case 1:
			resp = "Short answer"
		case 2:
			resp = "Longer and clearer answer"
		default:
			resp = "Another useful variation"
		}
		_, _ = w.Write([]byte(`{"choices":[{"text":"` + resp + `"}]}`))
	}))
	defer srv.Close()

	tool := New(srv.Client(), srv.URL, "test-model", "")

	args := map[string]any{
		"prompt":            "Explain self-consistency.",
		"parallel_requests": 3,
		"batch_size":        3,
		"aggregate":         true,
	}
	raw, _ := json.Marshal(args)
	outAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}

	out, ok := outAny.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", outAny)
	}
	if okVal, _ := out["ok"].(bool); !okVal {
		t.Fatalf("expected ok=true, got: %#v", out)
	}
	if got, _ := out["final_response"].(string); got != "Merged best answer" {
		t.Fatalf("expected synthesized final response, got %q", got)
	}
	if got, _ := out["aggregation_method"].(string); got != "synthesis" {
		t.Fatalf("expected aggregation_method=synthesis, got %q", got)
	}
	if gotCalls := atomic.LoadInt32(&calls); gotCalls != 3 {
		t.Fatalf("expected 3 endpoint calls max (2 parallel + 1 synthesis), got %d", gotCalls)
	}
}

func TestParallelCompletionsFallbackToBestCandidate(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		prompt, _ := body["prompt"].(string)
		if strings.Contains(prompt, "expert response aggregator") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		idx := atomic.AddInt32(&calls, 1)
		resp := "ok"
		if idx == 1 {
			resp = "tiny"
		}
		if idx == 2 {
			resp = "This candidate is more complete and includes more useful detail."
		}
		_, _ = w.Write([]byte(`{"choices":[{"text":"` + resp + `"}]}`))
	}))
	defer srv.Close()

	tool := New(srv.Client(), srv.URL, "test-model", "")
	args := map[string]any{
		"prompt":            "Describe the approach.",
		"parallel_requests": 2,
		"aggregate":         true,
	}
	raw, _ := json.Marshal(args)
	outAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	out := outAny.(map[string]any)
	if got, _ := out["aggregation_method"].(string); got != "best_candidate" {
		t.Fatalf("expected best_candidate fallback, got %q", got)
	}
	finalResp, _ := out["final_response"].(string)
	if finalResp != "This candidate is more complete and includes more useful detail." {
		t.Fatalf("unexpected fallback final response: %q", finalResp)
	}
}

func TestParallelCompletionsRequiresEndpointOrBaseURL(t *testing.T) {
	t.Parallel()

	tool := New(http.DefaultClient, "", "model", "")
	raw, _ := json.Marshal(map[string]any{"prompt": "hello"})
	outAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	out := outAny.(map[string]any)
	if okVal, _ := out["ok"].(bool); okVal {
		t.Fatalf("expected ok=false when endpoint missing: %#v", out)
	}
}

func TestParallelCompletionsAppliesDefaultMaxTokens(t *testing.T) {
	t.Parallel()

	var gotMaxTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["max_tokens"].(float64); ok {
			gotMaxTokens = int(v)
		}
		_, _ = w.Write([]byte(`{"choices":[{"text":"ok"}]}`))
	}))
	defer srv.Close()

	tool := New(srv.Client(), srv.URL, "test-model", "")
	raw, _ := json.Marshal(map[string]any{
		"prompt":            "Return complete code.",
		"parallel_requests": 1,
	})

	outAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	out := outAny.(map[string]any)
	if okVal, _ := out["ok"].(bool); !okVal {
		t.Fatalf("expected ok=true, got %#v", out)
	}
	if gotMaxTokens != defaultMaxTokens {
		t.Fatalf("expected default max_tokens %d, got %d", defaultMaxTokens, gotMaxTokens)
	}
}

func TestParallelCompletionsCapsParallelRequestsToThree(t *testing.T) {
	t.Parallel()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"choices":[{"text":"ok"}]}`))
	}))
	defer srv.Close()

	tool := New(srv.Client(), srv.URL, "test-model", "")
	raw, _ := json.Marshal(map[string]any{
		"prompt":            "test",
		"parallel_requests": 28,
		"aggregate":         false,
	})
	outAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	out := outAny.(map[string]any)
	if okVal, _ := out["ok"].(bool); !okVal {
		t.Fatalf("expected ok=true, got %#v", out)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected exactly 3 completion calls, got %d", got)
	}
}

func TestParallelCompletionsForcesMaxTokensTo16000(t *testing.T) {
	t.Parallel()

	var gotMaxTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if v, ok := body["max_tokens"].(float64); ok {
			gotMaxTokens = int(v)
		}
		_, _ = w.Write([]byte(`{"choices":[{"text":"ok"}]}`))
	}))
	defer srv.Close()

	tool := New(srv.Client(), srv.URL, "test-model", "")
	raw, _ := json.Marshal(map[string]any{
		"prompt":            "Return complete code.",
		"max_tokens":        32,
		"parallel_requests": 1,
	})

	outAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call() unexpected error: %v", err)
	}
	out := outAny.(map[string]any)
	if okVal, _ := out["ok"].(bool); !okVal {
		t.Fatalf("expected ok=true, got %#v", out)
	}
	if gotMaxTokens != 16000 {
		t.Fatalf("expected forced max_tokens 16000, got %d", gotMaxTokens)
	}
}
