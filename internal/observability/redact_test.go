package observability

import (
	"encoding/json"
	"testing"
)

func TestRedactJSON_SimpleAndNested(t *testing.T) {
	in := map[string]any{
		"api_key": "secret123",
		"user": map[string]any{
			"name":     "alice",
			"password": "hunter2",
		},
		"items": []any{
			map[string]any{"token": "tok"},
			"plain",
		},
		"note": "keepme",
	}
	b, _ := json.Marshal(in)
	out := RedactJSON(b)
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", v)
	}
	if m["api_key"] != "[REDACTED]" {
		t.Errorf("api_key not redacted: %v", m["api_key"])
	}
	user := m["user"].(map[string]any)
	if user["password"] != "[REDACTED]" {
		t.Errorf("nested password not redacted: %v", user["password"])
	}
	items := m["items"].([]any)
	first := items[0].(map[string]any)
	if first["token"] != "[REDACTED]" {
		t.Errorf("array nested token not redacted: %v", first["token"])
	}
	if m["note"] != "keepme" {
		t.Errorf("non-sensitive value mutated: %v", m["note"])
	}
}

func TestRedactJSON_EmptyAndInvalid(t *testing.T) {
	// Empty input should return as-is
	empty := json.RawMessage(nil)
	if got := RedactJSON(empty); got != nil {
		t.Errorf("expected nil raw for empty input, got %v", got)
	}

	// Invalid JSON should return original bytes
	raw := json.RawMessage([]byte("notjson"))
	res := RedactJSON(raw)
	if string(res) != "notjson" {
		t.Errorf("expected original bytes for invalid json, got %s", string(res))
	}
}

func TestRedactValueRedactsStructsMapsAndSlices(t *testing.T) {
	type nested struct {
		Authorization string `json:"authorization"`
		Visible       string `json:"visible"`
	}
	type payload struct {
		APIKey       string            `json:"apiKey"`
		Name         string            `json:"name"`
		ExtraHeaders map[string]string `json:"extraHeaders"`
		Nested       nested            `json:"nested"`
	}

	redacted, ok := RedactValue(payload{
		APIKey: "sk-secret",
		Name:   "keep",
		ExtraHeaders: map[string]string{
			"Authorization": "Bearer secret",
			"X-Trace":       "trace",
		},
		Nested: nested{Authorization: "Bearer nested", Visible: "shown"},
	}).(map[string]any)
	if !ok {
		t.Fatalf("expected redacted struct map, got %T", redacted)
	}
	if redacted["apiKey"] != RedactedValue {
		t.Fatalf("expected apiKey redacted, got %v", redacted["apiKey"])
	}
	if redacted["name"] != "keep" {
		t.Fatalf("expected name preserved, got %v", redacted["name"])
	}
	headers := redacted["extraHeaders"].(map[string]any)
	if headers["Authorization"] != RedactedValue || headers["X-Trace"] != "trace" {
		t.Fatalf("unexpected header redaction: %#v", headers)
	}
	n := redacted["nested"].(map[string]any)
	if n["authorization"] != RedactedValue || n["visible"] != "shown" {
		t.Fatalf("unexpected nested redaction: %#v", n)
	}
}
