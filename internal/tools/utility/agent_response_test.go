package utility

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAgentResponseToolCall(t *testing.T) {
	tool := NewAgentResponseTool()

	if tool.Name() != agentResponseToolName {
		t.Fatalf("unexpected name: %s", tool.Name())
	}

	schema := tool.JSONSchema()
	if schema["description"] == "" {
		t.Fatalf("expected description in schema")
	}

	args := map[string]any{
		"text":        "hello world",
		"render_mode": "html",
		"output_attr": "result",
		"label":       "Final",
	}
	raw, _ := json.Marshal(args)

	respAny, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	respMap, ok := respAny.(agentResponseResponse)
	if !ok {
		var fallback agentResponseResponse
		data, _ := json.Marshal(respAny)
		_ = json.Unmarshal(data, &fallback)
		respMap = fallback
	}
	if !respMap.OK {
		t.Fatalf("expected OK=true")
	}
	if respMap.Text != "hello world" {
		t.Fatalf("expected text hello world, got %q", respMap.Text)
	}
	if respMap.RenderMode != "html" {
		t.Fatalf("expected render_mode html, got %q", respMap.RenderMode)
	}
	if respMap.OutputAttr != "result" {
		t.Fatalf("expected output_attr result, got %q", respMap.OutputAttr)
	}
	if respMap.Label != "Final" {
		t.Fatalf("expected label Final, got %q", respMap.Label)
	}
}

func TestAgentResponseToolCallDefaults(t *testing.T) {
	tool := NewAgentResponseTool()

	respAny, err := tool.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	respMap, ok := respAny.(agentResponseResponse)
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
	if respMap.RenderMode != "markdown" {
		t.Fatalf("expected default render_mode markdown, got %q", respMap.RenderMode)
	}
}
