package openai

import (
	"encoding/json"
	"maps"
	"strings"

	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/shared"

	"manifold/internal/llm"
)

// removeUnsupportedSchema recursively deletes keys we know llama.cpp cannot
// handle (currently: "not") and returns the cleaned map.
func removeUnsupportedSchema(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	delete(in, "not")
	for k, v := range in {
		switch tv := v.(type) {
		case map[string]any:
			in[k] = removeUnsupportedSchema(tv)
		case []any:
			for idx, elem := range tv {
				if mm, ok := elem.(map[string]any); ok {
					tv[idx] = removeUnsupportedSchema(mm)
				}
			}
			in[k] = tv
		}
	}
	return in
}

// sanitizeToolSchemas clones and cleans tool schemas for self-hosted llama.cpp.
func sanitizeToolSchemas(src []llm.ToolSchema) []llm.ToolSchema {
	if len(src) == 0 {
		return src
	}
	out := make([]llm.ToolSchema, 0, len(src))
	for _, s := range src {
		if s.Parameters != nil {
			// shallow copy map to avoid mutating original
			cp := make(map[string]any, len(s.Parameters))
			maps.Copy(cp, s.Parameters)
			cleaned := removeUnsupportedSchema(cp)
			if len(cleaned) == 0 {
				s.Parameters = nil
			} else {
				s.Parameters = cleaned
			}
		}
		out = append(out, s)
	}
	return out
}

// ensureStrictJSONSchema enforces additionalProperties:false wherever a schema
// object is present. This matches stricter validation requirements of
// the Responses API for function tool parameters.
func ensureStrictJSONSchema(in any) any {
	switch v := in.(type) {
	case map[string]any:
		// If it looks like an object schema (has properties or type==object), force additionalProperties:false
		// Also ensure type: object is present when properties exist.
		if v["type"] == "object" || v["properties"] != nil || v["required"] != nil {
			v["additionalProperties"] = false
			if _, hasType := v["type"]; !hasType && v["properties"] != nil {
				v["type"] = "object"
			}
		}
		// Recurse into known schema containers
		if props, ok := v["properties"].(map[string]any); ok {
			for k, child := range props {
				props[k] = ensureStrictJSONSchema(child)
			}
			v["properties"] = props
		}
		if items, ok := v["items"]; ok {
			v["items"] = ensureStrictJSONSchema(items)
		}
		if allOf, ok := v["allOf"].([]any); ok {
			for i, child := range allOf {
				allOf[i] = ensureStrictJSONSchema(child)
			}
			v["allOf"] = allOf
		}
		if anyOf, ok := v["anyOf"].([]any); ok {
			for i, child := range anyOf {
				anyOf[i] = ensureStrictJSONSchema(child)
			}
			v["anyOf"] = anyOf
		}
		if oneOf, ok := v["oneOf"].([]any); ok {
			for i, child := range oneOf {
				oneOf[i] = ensureStrictJSONSchema(child)
			}
			v["oneOf"] = oneOf
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = ensureStrictJSONSchema(child)
		}
		return v
	default:
		return in
	}
}

// extractReasoningEffort copies the reasoning_effort flag into the typed
// Responses.Reasoning field and removes the deprecated top-level parameter so
// it is not sent to the API.
func extractReasoningEffort(extra map[string]any) (shared.ReasoningEffort, bool) {
	if extra == nil {
		return "", false
	}
	var raw any
	var ok bool
	found := false
	if raw, ok = extra["reasoning_effort"]; ok {
		delete(extra, "reasoning_effort")
		found = true
	} else if raw, ok = extra["reasoningEffort"]; ok {
		delete(extra, "reasoningEffort")
		found = true
	}

	// Also support reasoning.effort and ensure it doesn't override the typed field.
	if rmRaw, hasReasoning := extra["reasoning"]; hasReasoning {
		if rm, ok := rmRaw.(map[string]any); ok {
			if val, hasEffort := rm["effort"]; hasEffort {
				if !found {
					raw = val
					found = true
				}
				delete(rm, "effort")
			}
			if len(rm) == 0 {
				delete(extra, "reasoning")
			} else {
				extra["reasoning"] = rm
			}
		}
	}

	if !found {
		return "", false
	}
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return shared.ReasoningEffort(s), true
}

func extractReasoningSummary(extra map[string]any) (shared.ReasoningSummary, bool) {
	if extra == nil {
		return "", false
	}
	// Back-compat: some configs used a top-level "summary" parameter. The Responses API
	// does not accept this; map it into reasoning.summary and ensure it is not sent
	// as an extra field.
	if raw, ok := extra["summary"]; ok {
		delete(extra, "summary")
		if s, ok := raw.(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return shared.ReasoningSummary(trimmed), true
			}
		}
		return "", false
	}
	raw, ok := extra["reasoning_summary"]
	if !ok {
		raw, ok = extra["reasoningSummary"]
	}
	if ok {
		delete(extra, "reasoning_summary")
		delete(extra, "reasoningSummary")
		if s, ok := raw.(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				return shared.ReasoningSummary(trimmed), true
			}
		}
		return "", false
	}
	raw, ok = extra["reasoning"]
	if !ok {
		return "", false
	}
	rm, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	val, ok := rm["summary"]
	if !ok {
		return "", false
	}
	delete(rm, "summary")
	if len(rm) == 0 {
		delete(extra, "reasoning")
	} else {
		extra["reasoning"] = rm
	}
	if s, ok := val.(string); ok {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			return shared.ReasoningSummary(trimmed), true
		}
	}
	return "", false
}

func configureChatCompletionTools(params *sdk.ChatCompletionNewParams, tools []llm.ToolSchema, selfHosted bool) bool {
	if params == nil {
		return false
	}
	if hasNativeWebSearchSchema(tools) {
		params.WebSearchOptions = sdk.ChatCompletionNewParamsWebSearchOptions{
			SearchContextSize: "medium",
		}
	}
	if len(tools) == 0 {
		return false
	}
	schemas := tools
	if selfHosted {
		schemas = sanitizeToolSchemas(tools)
	}
	adapted := AdaptSchemas(schemas)
	if len(adapted) == 0 {
		return false
	}
	params.Tools = adapted
	return true
}

func configureChatCompletionBodyTools(body map[string]any, tools []llm.ToolSchema, selfHosted bool) bool {
	if body == nil {
		return false
	}
	if hasNativeWebSearchSchema(tools) {
		body["web_search_options"] = map[string]any{"search_context_size": "medium"}
	}
	if len(tools) == 0 {
		return false
	}
	schemas := tools
	if selfHosted {
		schemas = sanitizeToolSchemas(tools)
	}
	adapted := AdaptSchemas(schemas)
	if len(adapted) == 0 {
		return false
	}
	body["tools"] = adapted
	return true
}

func (c *Client) requestTools(tools []llm.ToolSchema) []llm.ToolSchema {
	if c == nil || c.isSelfHosted() || len(tools) == 0 {
		return tools
	}
	return appendNativeWebSearchSchema(tools)
}

// isEmptyArgs reports whether the provided arguments string is effectively empty
// (blank, null, or an empty JSON object/array). This guards against ghost tool
// calls with missing payloads.
func isEmptyArgs(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" || s == "{}" || s == "[]" {
		return true
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		// If it's invalid JSON but non-empty, let the tool handle the error.
		return false
	}
	switch t := v.(type) {
	case map[string]any:
		return len(t) == 0
	case []any:
		return len(t) == 0
	case string:
		return strings.TrimSpace(t) == ""
	}
	return false
}

func isEmptyArgsBytes(raw []byte) bool {
	return isEmptyArgs(string(raw))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
