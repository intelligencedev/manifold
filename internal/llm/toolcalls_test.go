package llm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeToolCallsSplitsConcatenatedJSONObjectArguments(t *testing.T) {
	calls := []ToolCall{{
		Name: "web_fetch",
		ID:   "call_fetch",
		Args: json.RawMessage(`{"url":"https://one.example","timeout_seconds":15}{"url":"https://two.example","timeout_seconds":15}`),
	}}

	normalized := NormalizeToolCalls(calls)

	if len(normalized) != 2 {
		t.Fatalf("expected two split calls, got %#v", normalized)
	}
	if normalized[0].ID != "call_fetch" {
		t.Fatalf("expected first call to keep original id, got %q", normalized[0].ID)
	}
	if normalized[1].ID != "call_fetch_split_2" {
		t.Fatalf("expected second call to receive split id, got %q", normalized[1].ID)
	}
	if !json.Valid(normalized[0].Args) || !json.Valid(normalized[1].Args) {
		t.Fatalf("expected valid split JSON args, got %q and %q", normalized[0].Args, normalized[1].Args)
	}
	if string(normalized[0].Args) != `{"url":"https://one.example","timeout_seconds":15}` {
		t.Fatalf("unexpected first args %s", normalized[0].Args)
	}
	if string(normalized[1].Args) != `{"url":"https://two.example","timeout_seconds":15}` {
		t.Fatalf("unexpected second args %s", normalized[1].Args)
	}
}

func TestNormalizeToolCallsLeavesSingleJSONObjectArgumentsAlone(t *testing.T) {
	calls := []ToolCall{{
		Name: "web_fetch",
		ID:   "call_fetch",
		Args: json.RawMessage(`{"url":"https://one.example","timeout_seconds":15}`),
	}}

	normalized := NormalizeToolCalls(calls)

	if len(normalized) != 1 {
		t.Fatalf("expected one call, got %#v", normalized)
	}
	if string(normalized[0].Args) != string(calls[0].Args) {
		t.Fatalf("expected args unchanged, got %s", normalized[0].Args)
	}
}

func TestNormalizeToolCallsLeavesUnrecoverableArgumentsAlone(t *testing.T) {
	calls := []ToolCall{{
		Name: "web_fetch",
		ID:   "call_fetch",
		Args: json.RawMessage(`{"url":"https://one.example"} trailing`),
	}}

	normalized := NormalizeToolCalls(calls)

	if len(normalized) != 1 {
		t.Fatalf("expected one call, got %#v", normalized)
	}
	if string(normalized[0].Args) != string(calls[0].Args) {
		t.Fatalf("expected args unchanged, got %s", normalized[0].Args)
	}
}
