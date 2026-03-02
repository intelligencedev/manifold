package llm

import "testing"

func TestNormalizeExtraParams_DecodesStructuredJSONStringValues(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"thinking":      `{"type":"adaptive"}`,
		"output_config": `{"effort":"medium"}`,
	}

	got := NormalizeExtraParams(in)

	thinking, ok := got["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking type = %T, want map[string]any", got["thinking"])
	}
	if thinking["type"] != "adaptive" {
		t.Fatalf("thinking.type = %v, want adaptive", thinking["type"])
	}

	outputCfg, ok := got["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config type = %T, want map[string]any", got["output_config"])
	}
	if outputCfg["effort"] != "medium" {
		t.Fatalf("output_config.effort = %v, want medium", outputCfg["effort"])
	}
}

func TestNormalizeExtraParams_LeavesPlainStringsUntouched(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"reasoning_effort": "medium",
		"note":             "keep as text",
	}

	got := NormalizeExtraParams(in)

	if got["reasoning_effort"] != "medium" {
		t.Fatalf("reasoning_effort = %v, want medium", got["reasoning_effort"])
	}
	if got["note"] != "keep as text" {
		t.Fatalf("note = %v, want keep as text", got["note"])
	}
}

func TestNormalizeExtraParams_DecodesNestedStructuredJSON(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"wrapper": map[string]any{
			"thinking": `{"type":"adaptive"}`,
		},
	}

	got := NormalizeExtraParams(in)

	wrapper, ok := got["wrapper"].(map[string]any)
	if !ok {
		t.Fatalf("wrapper type = %T, want map[string]any", got["wrapper"])
	}
	thinking, ok := wrapper["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("wrapper.thinking type = %T, want map[string]any", wrapper["thinking"])
	}
	if thinking["type"] != "adaptive" {
		t.Fatalf("wrapper.thinking.type = %v, want adaptive", thinking["type"])
	}
}
