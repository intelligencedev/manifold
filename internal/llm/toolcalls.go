package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// NormalizeToolCalls repairs provider tool-call shapes that are valid enough to
// recover deterministically. Some local OpenAI-compatible servers occasionally
// concatenate multiple JSON argument objects into one function call argument
// string. Split those into separate calls so tools receive valid JSON and the
// next provider request can replay a valid tool-call transcript.
func NormalizeToolCalls(calls []ToolCall) []ToolCall {
	if len(calls) == 0 {
		return calls
	}
	out := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		parts := splitConcatenatedJSONObjectArgs(call.Args)
		if len(parts) <= 1 {
			out = append(out, call)
			continue
		}
		baseID := strings.TrimSpace(call.ID)
		for i, part := range parts {
			next := call
			next.Args = part
			if baseID != "" && i > 0 {
				next.ID = fmt.Sprintf("%s_split_%d", baseID, i+1)
			}
			out = append(out, next)
		}
	}
	return out
}

func splitConcatenatedJSONObjectArgs(raw json.RawMessage) []json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	parts := make([]json.RawMessage, 0, 2)
	for {
		start := skipJSONWhitespace(trimmed, int(dec.InputOffset()))
		if start >= len(trimmed) {
			break
		}
		if trimmed[start] != '{' {
			return nil
		}
		var decoded map[string]any
		if err := dec.Decode(&decoded); err != nil {
			return nil
		}
		end := int(dec.InputOffset())
		if end <= start || end > len(trimmed) {
			return nil
		}
		part := bytes.TrimSpace(trimmed[start:end])
		if !json.Valid(part) {
			return nil
		}
		parts = append(parts, append(json.RawMessage(nil), part...))
	}
	if len(parts) <= 1 {
		return nil
	}
	return parts
}

func skipJSONWhitespace(raw []byte, start int) int {
	for start < len(raw) {
		switch raw[start] {
		case ' ', '\n', '\r', '\t':
			start++
		default:
			return start
		}
	}
	return start
}
