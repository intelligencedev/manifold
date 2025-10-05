package utility

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTextboxToolCall(t *testing.T) {
	tool := NewTextboxTool()

	if tool.Name() != textboxToolName {
		t.Fatalf("unexpected name: %s", tool.Name())
	}

	schema := tool.JSONSchema()
	if schema["description"] == "" {
		t.Fatalf("expected description in schema")
	}

	args := map[string]any{"text": "hello", "output_attr": "greeting", "label": "Intro"}
	raw, _ := json.Marshal(args)

	respAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	respMap, ok := respAny.(textboxResponse)
	if !ok {
		// Allow map if json.Unmarshal path changes in future.
		var fallback textboxResponse
		data, _ := json.Marshal(respAny)
		_ = json.Unmarshal(data, &fallback)
		respMap = fallback
	}
	if respMap.Text != "hello" {
		t.Fatalf("expected text hello, got %q", respMap.Text)
	}
	if respMap.OutputAttr != "greeting" {
		t.Fatalf("expected output_attr greeting, got %q", respMap.OutputAttr)
	}
	if respMap.Label != "Intro" {
		t.Fatalf("expected label Intro, got %q", respMap.Label)
	}
}

func TestTextboxToolCallDefaults(t *testing.T) {
	tool := NewTextboxTool()

	respAny, err := tool.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	respMap, ok := respAny.(textboxResponse)
	if !ok {
		data, _ := json.Marshal(respAny)
		_ = json.Unmarshal(data, &respMap)
	}
	if !respMap.OK {
		t.Fatalf("expected OK=true")
	}
	if respMap.Text != "" {
		t.Fatalf("expected empty text by default, got %q", respMap.Text)
	}
	if respMap.OutputAttr != "" {
		t.Fatalf("expected empty output attr by default, got %q", respMap.OutputAttr)
	}
}
