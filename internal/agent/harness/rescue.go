package harness

import (
	"bytes"
	"encoding/json"
	"strings"

	"manifold/internal/llm"
)

// RescueEmbeddedToolCalls converts structured tool-call JSON embedded in text
// into provider-native tool calls. It is intentionally conservative: only
// valid JSON shapes are rescued, and normal prose is left untouched.
func RescueEmbeddedToolCalls(msg llm.Message, enabled bool) (llm.Message, bool) {
	if !enabled || len(msg.ToolCalls) > 0 {
		return msg, false
	}
	for _, candidate := range embeddedJSONCandidates(msg.Content) {
		calls := parseEmbeddedToolCalls(candidate)
		if len(calls) == 0 {
			continue
		}
		msg.ToolCalls = calls
		msg.Content = ""
		return msg, true
	}
	return msg, false
}

func embeddedJSONCandidates(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	candidates := []string{content}
	if fenced := firstFencedBlock(content); fenced != "" {
		candidates = append(candidates, fenced)
	}
	if object := firstJSONObject(content); object != "" && object != content {
		candidates = append(candidates, object)
	}
	return candidates
}

func firstFencedBlock(content string) string {
	start := strings.Index(content, "```")
	if start < 0 {
		return ""
	}
	rest := content[start+3:]
	if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
		rest = rest[newline+1:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func firstJSONObject(content string) string {
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(content[start : end+1])
}

func parseEmbeddedToolCalls(raw string) []llm.ToolCall {
	rawBytes := bytes.TrimSpace([]byte(raw))
	if len(rawBytes) == 0 || rawBytes[0] != '{' {
		return nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(rawBytes, &root); err != nil {
		return nil
	}

	if rawCalls := firstRaw(root, "tool_calls", "toolCalls"); len(rawCalls) > 0 {
		var entries []map[string]json.RawMessage
		if err := json.Unmarshal(rawCalls, &entries); err == nil {
			return parseEmbeddedToolCallList(entries)
		}
	}
	if call := parseEmbeddedToolCall(root); call.Name != "" {
		return []llm.ToolCall{call}
	}
	return nil
}

func parseEmbeddedToolCallList(entries []map[string]json.RawMessage) []llm.ToolCall {
	calls := make([]llm.ToolCall, 0, len(entries))
	for _, entry := range entries {
		call := parseEmbeddedToolCall(entry)
		if call.Name == "" {
			continue
		}
		calls = append(calls, call)
	}
	return calls
}

func parseEmbeddedToolCall(entry map[string]json.RawMessage) llm.ToolCall {
	name := rawString(firstRaw(entry, "name", "tool", "tool_name", "toolName"))
	if name == "" {
		return llm.ToolCall{}
	}
	args := firstRaw(entry, "args", "arguments", "input")
	if len(args) == 0 || bytes.Equal(bytes.TrimSpace(args), []byte("null")) {
		args = json.RawMessage(`{}`)
	}
	trimmedArgs := bytes.TrimSpace(args)
	if len(trimmedArgs) == 0 {
		args = json.RawMessage(`{}`)
		trimmedArgs = args
	}
	if trimmedArgs[0] == '"' {
		var argString string
		if err := json.Unmarshal(args, &argString); err == nil {
			args = json.RawMessage(argString)
		}
	}
	if !json.Valid(args) {
		args = json.RawMessage(`{}`)
	}
	return llm.ToolCall{
		Name: strings.TrimSpace(name),
		ID:   rawString(firstRaw(entry, "id", "tool_call_id", "toolCallID")),
		Args: append(json.RawMessage(nil), args...),
	}
}

func firstRaw(entry map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		if raw, ok := entry[key]; ok {
			return raw
		}
	}
	return nil
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}
