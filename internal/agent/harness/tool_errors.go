package harness

import (
	"encoding/json"
	"strings"
)

// ToolError describes a structured tool execution failure.
type ToolError struct {
	Message       string
	PolicyBlocked bool
}

// ToolErrorTracker counts consecutive tool failures and resets on success.
type ToolErrorTracker struct {
	max         int
	consecutive int
}

// NewToolErrorTracker creates a bounded tool-error tracker.
func NewToolErrorTracker(max int) *ToolErrorTracker {
	if max <= 0 {
		max = defaultMaxToolErrors
	}
	return &ToolErrorTracker{max: max}
}

// RecordSuccess clears consecutive tool-error state.
func (t *ToolErrorTracker) RecordSuccess() {
	if t == nil {
		return
	}
	t.consecutive = 0
}

// RecordFailure increments consecutive tool-error state and reports exhaustion.
func (t *ToolErrorTracker) RecordFailure(tool string, err ToolError) (int, error) {
	if t == nil {
		t = NewToolErrorTracker(defaultMaxToolErrors)
	}
	t.consecutive++
	if t.consecutive >= t.max {
		return t.consecutive, ToolErrorsExhaustedError{ToolName: tool, Count: t.consecutive, Last: err}
	}
	return t.consecutive, nil
}

// Consecutive returns the current consecutive failure count.
func (t *ToolErrorTracker) Consecutive() int {
	if t == nil {
		return 0
	}
	return t.consecutive
}

// DetectToolErrorPayload recognizes common Manifold tool error payload shapes.
func DetectToolErrorPayload(payload []byte) (ToolError, bool) {
	payload = []byte(strings.TrimSpace(string(payload)))
	if len(payload) == 0 {
		return ToolError{}, false
	}

	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return ToolError{}, false
	}

	if ok, exists := obj["ok"].(bool); exists && ok {
		return ToolError{}, false
	}

	policyBlocked := false
	if _, exists := obj["policy_id"]; exists {
		policyBlocked = true
	}

	if ok, exists := obj["ok"].(bool); exists && !ok {
		return ToolError{
			Message:       errorMessageFromObject(obj),
			PolicyBlocked: policyBlocked || strings.Contains(errorMessageFromObject(obj), "blocked by policy"),
		}, true
	}

	if msg := errorString(obj["error"]); msg != "" {
		return ToolError{Message: msg, PolicyBlocked: policyBlocked || strings.Contains(msg, "blocked by policy")}, true
	}

	return ToolError{}, false
}

func errorMessageFromObject(obj map[string]any) string {
	if msg := errorString(obj["error"]); msg != "" {
		return msg
	}
	if msg := errorString(obj["message"]); msg != "" {
		return msg
	}
	return "tool execution failed"
}

func errorString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
