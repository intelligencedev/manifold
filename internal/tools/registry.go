package tools

import (
	"context"
	"encoding/json"

	"gptagent/internal/llm"
)

// DispatchEvent captures a single tool dispatch invocation and result.
type DispatchEvent struct {
	Name    string
	Args    json.RawMessage
	Payload []byte
	Err     error
}

type recordingRegistry struct {
	base Registry
	on   func(DispatchEvent)
}

// NewRecordingRegistry wraps an existing Registry and calls on for each Dispatch.
func NewRecordingRegistry(base Registry, on func(DispatchEvent)) Registry {
	if base == nil {
		base = NewRegistry()
	}
	return &recordingRegistry{base: base, on: on}
}

func (r *recordingRegistry) Register(t Tool)           { r.base.Register(t) }
func (r *recordingRegistry) Schemas() []llm.ToolSchema { return r.base.Schemas() }

// We need to mirror Schemas returning []llm.ToolSchema; to avoid import cycle,
// delegate directly since base implements it. This adapter method signature is
// resolved by the interface at compile time.
func (r *recordingRegistry) Dispatch(ctx context.Context, name string, raw json.RawMessage) ([]byte, error) {
	payload, err := r.base.Dispatch(ctx, name, raw)
	if r.on != nil {
		r.on(DispatchEvent{Name: name, Args: raw, Payload: payload, Err: err})
	}
	return payload, err
}
