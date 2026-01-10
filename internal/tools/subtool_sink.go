package tools

import (
	"context"
	"encoding/json"
)

// Subtool sink plumbing to surface nested tool calls (e.g., multi_tool_use_parallel subcalls)
// to UIs and logs without relying on provider-level tool call streaming.
type subtoolSinkKey struct{}

// SubtoolEvent describes a lifecycle event for a subtool execution.
// Phase is "start" or "end"; Args are the tool arguments; Payload is the tool result
// (only for phase=end); Error contains an error string if the subtool failed; DurationMS
// may be set by the caller for end events; ToolCallID can be provided by the wrapper input.
type SubtoolEvent struct {
	Phase      string          `json:"phase"`
	Name       string          `json:"name"`
	Args       json.RawMessage `json:"args,omitempty"`
	Payload    []byte          `json:"payload,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// SubtoolSink receives subtool lifecycle events.
type SubtoolSink func(SubtoolEvent)

// WithSubtoolSink attaches a sink to the context so tools can emit subtool events.
func WithSubtoolSink(ctx context.Context, s SubtoolSink) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, subtoolSinkKey{}, s)
}

// SubtoolSinkFromContext returns a previously set SubtoolSink, or nil.
func SubtoolSinkFromContext(ctx context.Context) SubtoolSink {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(subtoolSinkKey{}); v != nil {
		if s, ok := v.(SubtoolSink); ok {
			return s
		}
	}
	return nil
}
