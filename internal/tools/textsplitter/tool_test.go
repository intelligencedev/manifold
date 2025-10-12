package textsplitter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONSchemaParametersCompliant(t *testing.T) {
	tool := New()
	schema := tool.JSONSchema()
	params, ok := schema["parameters"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", params["type"])
	for _, key := range []string{"oneOf", "anyOf", "allOf", "enum", "not"} {
		_, exists := params[key]
		require.Falsef(t, exists, "schema unexpectedly contains %s", key)
	}
}

func TestSplitTextCharsAndDefaults(t *testing.T) {
	tool := New()
	text := "abcdefghij" // 10 chars
	// Use defaults: kind=fixed, unit=chars, size defaults to 100 -> single chunk
	payload, err := json.Marshal(map[string]any{"text": text})
	require.NoError(t, err)
	resAny, err := tool.Call(context.Background(), payload)
	require.NoError(t, err)
	res, ok := resAny.(map[string]any)
	require.True(t, ok)
	// JSON unmarshal into interface{} won't keep []string directly; re-marshal to normalize
	b, _ := json.Marshal(res["chunks"]) // []any
	var norm []string
	_ = json.Unmarshal(b, &norm)
	require.Equal(t, []string{text}, norm)

	// Now request smaller size with overlap
	payload, err = json.Marshal(map[string]any{
		"text":    text,
		"kind":    "fixed",
		"unit":    "chars",
		"size":    4,
		"overlap": 1,
	})
	require.NoError(t, err)
	resAny, err = tool.Call(context.Background(), payload)
	require.NoError(t, err)
	res, ok = resAny.(map[string]any)
	require.True(t, ok)
	b, _ = json.Marshal(res["chunks"]) // []any
	norm = nil
	_ = json.Unmarshal(b, &norm)
	// size=4, overlap=1 -> step=3 over 10 chars: [0:4],[3:7],[6:10]
	require.Equal(t, []string{"abcd", "defg", "ghij"}, norm)
}

func TestSplitTextTokensWhitespace(t *testing.T) {
	tool := New()
	text := "one two three four five six"
	payload, err := json.Marshal(map[string]any{
		"text": text,
		"unit": "tokens",
		"size": 2,
	})
	require.NoError(t, err)
	resAny, err := tool.Call(context.Background(), payload)
	require.NoError(t, err)
	res, ok := resAny.(map[string]any)
	require.True(t, ok)
	b, _ := json.Marshal(res["chunks"]) // []any
	var norm []string
	_ = json.Unmarshal(b, &norm)
	require.Equal(t, []string{"one two", "three four", "five six"}, norm)
}

func TestKindsEnumerated(t *testing.T) {
    tool := New()
    schema := tool.JSONSchema()
    props := schema["parameters"].(map[string]any)["properties"].(map[string]any)
    kindsAny := props["kind"].(map[string]any)["enum"].([]any)
    // ensure a few expected kinds exist
    want := map[string]bool{"fixed": true, "sentences": true, "paragraphs": true, "markdown": true, "code": true, "semantic": true, "texttiling": true, "rolling_sentences": true, "hybrid": true, "layout": true, "recursive": true}
    have := map[string]bool{}
    for _, k := range kindsAny {
        have[k.(string)] = true
    }
    for k := range want {
        if !have[k] {
            t.Fatalf("missing kind %s", k)
        }
    }
}

func TestSentenceChunkingBasic(t *testing.T) {
    tool := New()
    text := "One. Two three. Four five six."
    payload, _ := json.Marshal(map[string]any{"text": text, "kind": "sentences", "unit": "chars", "size": 20})
    resAny, err := tool.Call(context.Background(), payload)
    require.NoError(t, err)
    res := resAny.(map[string]any)
    b, _ := json.Marshal(res["chunks"]) 
    var chunks []string
    _ = json.Unmarshal(b, &chunks)
    require.GreaterOrEqual(t, len(chunks), 2)
}
