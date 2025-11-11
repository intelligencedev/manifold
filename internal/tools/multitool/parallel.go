package multitool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"manifold/internal/tools"
)

const (
	defaultMaxParallel = 8
	ToolName           = "multi_tool_use_parallel"
)

// Option allows configuring the parallel tool.
type Option func(*ParallelTool)

// WithMaxParallel caps the number of concurrent tool executions. Non-positive
// values default to the number of requested tool uses.
func WithMaxParallel(n int) Option {
	return func(t *ParallelTool) {
		t.maxParallel = n
	}
}

// ParallelTool implements multi_tool_use_parallel, dispatching multiple tool
// calls concurrently using the provided tools registry.
type ParallelTool struct {
	mu          sync.RWMutex
	registry    tools.Registry
	maxParallel int
}

// NewParallel creates a ParallelTool bound to the provided registry view.
func NewParallel(reg tools.Registry, opts ...Option) *ParallelTool {
	pt := &ParallelTool{
		registry:    reg,
		maxParallel: defaultMaxParallel,
	}
	for _, opt := range opts {
		opt(pt)
	}
	return pt
}

// SetRegistry updates the registry view used for dispatching tool calls. This
// is useful when the caller swaps in a filtered registry after initial
// construction while keeping the same ParallelTool instance registered.
func (t *ParallelTool) SetRegistry(reg tools.Registry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.registry = reg
}

// Name returns the registered tool name.
func (t *ParallelTool) Name() string { return ToolName }

// JSONSchema describes the expected input arguments.
func (t *ParallelTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Run multiple functions tools concurrently when their work is independent.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool_uses": map[string]any{
					"type":        "array",
					"description": "List of tool invocations to execute in parallel.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"recipient_name": map[string]any{
								"type":        "string",
								"description": "Tool identifier (e.g., functions.web_fetch).",
							},
							"parameters": map[string]any{
								"type":        "object",
								"description": "JSON arguments for the tool.",
							},
							"tool_call_id": map[string]any{
								"type":        "string",
								"description": "Optional identifier to correlate results.",
							},
						},
						"required": []string{"recipient_name"},
					},
					"minItems": 1,
					"maxItems": 32,
				},
				"timeout_ms": map[string]any{
					"type":        "integer",
					"description": "Optional timeout applied to each tool call in milliseconds.",
					"minimum":     1,
				},
			},
			"required": []string{"tool_uses"},
		},
	}
}

type parallelCall struct {
	RecipientName string          `json:"recipient_name"`
	Parameters    json.RawMessage `json:"parameters"`
	ToolCallID    string          `json:"tool_call_id"`
}

type parallelArgs struct {
	ToolUses  []parallelCall `json:"tool_uses"`
	TimeoutMS int            `json:"timeout_ms"`
}

type callResult struct {
	RecipientName string          `json:"recipient_name"`
	ToolName      string          `json:"tool_name"`
	ToolCallID    string          `json:"tool_call_id,omitempty"`
	DurationMS    int64           `json:"duration_ms"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Error         string          `json:"error,omitempty"`
}

// Call executes the configured tool uses concurrently and aggregates the
// payloads. Each tool call inherits the provided context and optional timeout.
func (t *ParallelTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{"ok": false, "error": "tool_uses required"}, nil
	}

	args, err := parseArgs(raw)
	if err != nil {
		return nil, err
	}
	reg := t.registryView()
	if reg == nil {
		return map[string]any{"ok": false, "error": "tool registry unavailable"}, nil
	}
	if len(args.ToolUses) == 0 {
		return map[string]any{"ok": false, "error": "tool_uses must contain at least one entry"}, nil
	}
	if len(args.ToolUses) > 32 {
		return map[string]any{"ok": false, "error": "tool_uses exceeds maximum of 32"}, nil
	}

	timeout := time.Duration(args.TimeoutMS) * time.Millisecond

	// Subtool observability sink, if present in context
	sink := tools.SubtoolSinkFromContext(ctx)

	results := make([]callResult, len(args.ToolUses))
	var (
		errs []string
		mu   sync.Mutex
	)
	var wg sync.WaitGroup

	maxParallel := t.maxParallel
	if maxParallel <= 0 || maxParallel > len(args.ToolUses) {
		maxParallel = len(args.ToolUses)
	}
	sem := make(chan struct{}, maxParallel)

	for idx, call := range args.ToolUses {
		toolName, err := normalizeRecipient(call.RecipientName)
		if err != nil {
			return map[string]any{
				"ok":    false,
				"error": fmt.Sprintf("invalid recipient_name (%s): %v", call.RecipientName, err),
			}, nil
		}
		if toolName == t.Name() {
			msg := "recursive multi_tool_use_parallel invocation is not allowed"
			return map[string]any{
				"ok":    false,
				"error": msg,
			}, nil
		}

		wg.Add(1)
		go func(i int, spec parallelCall, name string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errMsg := ctx.Err().Error()
				results[i] = callResult{
					RecipientName: spec.RecipientName,
					ToolName:      name,
					ToolCallID:    spec.ToolCallID,
					Error:         errMsg,
				}
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s: %s", name, errMsg))
				mu.Unlock()
				return
			}
			defer func() { <-sem }()

			argsPayload := spec.Parameters
			if len(argsPayload) == 0 || string(argsPayload) == "null" {
				argsPayload = []byte("{}")
			}

			dispatchCtx := ctx
			if timeout > 0 {
				var cancel context.CancelFunc
				dispatchCtx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			// Emit a subtool start event for observability
			if sink != nil {
				sink(tools.SubtoolEvent{Phase: "start", Name: name, Args: argsPayload, ToolCallID: spec.ToolCallID})
			}

			start := time.Now()
			payload, err := reg.Dispatch(dispatchCtx, name, argsPayload)
			elapsed := time.Since(start)
			res := callResult{
				RecipientName: spec.RecipientName,
				ToolName:      name,
				ToolCallID:    spec.ToolCallID,
				DurationMS:    elapsed.Milliseconds(),
			}
			if err != nil {
				res.Error = err.Error()
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
				mu.Unlock()
			} else {
				if len(payload) == 0 {
					payload = []byte("null")
				}
				if embeddedErr := detectEmbeddedError(payload); embeddedErr != "" {
					res.Error = embeddedErr
					mu.Lock()
					errs = append(errs, fmt.Sprintf("%s: %s", name, embeddedErr))
					mu.Unlock()
				}
				cp := make([]byte, len(payload))
				copy(cp, payload)
				res.Payload = json.RawMessage(cp)
			}
			// Emit a subtool end event for observability
			if sink != nil {
				var pay []byte
				if res.Payload != nil {
					pay = []byte(res.Payload)
				}
				sink(tools.SubtoolEvent{Phase: "end", Name: name, Args: argsPayload, Payload: pay, Error: res.Error, DurationMS: res.DurationMS, ToolCallID: spec.ToolCallID})
			}

			results[i] = res
		}(idx, call, toolName)
	}

	wg.Wait()

	ok := len(errs) == 0
	resp := map[string]any{
		"ok":      ok,
		"results": results,
	}
	if !ok {
		resp["error"] = strings.Join(errs, "; ")
	}
	return resp, nil
}

func (t *ParallelTool) registryView() tools.Registry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.registry
}

func normalizeRecipient(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("recipient_name is empty")
	}
	if strings.HasPrefix(v, "functions.") {
		v = strings.TrimPrefix(v, "functions.")
	}
	if v == "multi_tool_use.parallel" {
		v = ToolName
	}
	if strings.TrimSpace(v) == "" {
		return "", errors.New("recipient_name missing tool identifier")
	}
	return v, nil
}

func parseArgs(raw json.RawMessage) (parallelArgs, error) {
	var wrapper struct {
		ToolUses  json.RawMessage `json:"tool_uses"`
		TimeoutMS int             `json:"timeout_ms"`
	}

	if err := json.Unmarshal(raw, &wrapper); err == nil && len(bytes.TrimSpace(wrapper.ToolUses)) > 0 {
		calls, err := parseCallList(wrapper.ToolUses)
		if err != nil {
			return parallelArgs{}, err
		}
		return parallelArgs{ToolUses: calls, TimeoutMS: wrapper.TimeoutMS}, nil
	}

	calls, err := parseCallList(raw)
	if err != nil {
		return parallelArgs{}, err
	}
	return parallelArgs{ToolUses: calls, TimeoutMS: wrapper.TimeoutMS}, nil
}

func parseCallList(raw []byte) ([]parallelCall, error) {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 {
		return nil, errors.New("tool_uses required")
	}

	if data[0] == '[' {
		var nodes []json.RawMessage
		if err := json.Unmarshal(data, &nodes); err != nil {
			return nil, err
		}
		if len(nodes) == 0 {
			return nil, errors.New("tool_uses must contain at least one entry")
		}
		out := make([]parallelCall, 0, len(nodes))
		for _, node := range nodes {
			call, err := decodeCall(node)
			if err != nil {
				return nil, err
			}
			out = append(out, call)
		}
		return out, nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	out := make([]parallelCall, 0, 4)
	for {
		var node json.RawMessage
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		call, err := decodeCall(node)
		if err != nil {
			return nil, err
		}
		out = append(out, call)
	}
	if len(out) == 0 {
		return nil, errors.New("no tool uses parsed")
	}
	return out, nil
}

func decodeCall(raw json.RawMessage) (parallelCall, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return parallelCall{}, err
	}

	var call parallelCall
	call.RecipientName = readString(envelope, "recipient_name")
	if call.RecipientName == "" {
		call.RecipientName = readString(envelope, "name")
	}
	if call.RecipientName == "" {
		call.RecipientName = readString(envelope, "tool")
	}
	if call.RecipientName == "" {
		call.RecipientName = readString(envelope, "tool_name")
	}

	if params, ok := envelope["parameters"]; ok && len(bytes.TrimSpace(params)) > 0 {
		call.Parameters = params
	} else if args, ok := envelope["arguments"]; ok && len(bytes.TrimSpace(args)) > 0 {
		call.Parameters = args
	}

	call.ToolCallID = readString(envelope, "tool_call_id")
	if call.ToolCallID == "" {
		call.ToolCallID = readString(envelope, "id")
	}

	if call.RecipientName == "" {
		// Detect implicit run_cli payloads generated by models (command/args pairs).
		if _, hasCommand := envelope["command"]; hasCommand {
			call.RecipientName = "run_cli"
		} else if _, hasCommands := envelope["commands"]; hasCommands {
			call.RecipientName = "run_cli"
		}
	}

	if call.Parameters == nil {
		if params := synthesizeParameters(envelope); params != nil {
			call.Parameters = params
		}
	}

	if call.Parameters == nil {
		call.Parameters = []byte("{}")
	}
	if strings.TrimSpace(call.RecipientName) == "" {
		return parallelCall{}, errors.New("recipient_name is empty")
	}
	return call, nil
}

func readString(src map[string]json.RawMessage, key string) string {
	raw, ok := src[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func synthesizeParameters(envelope map[string]json.RawMessage) json.RawMessage {
	keys := []string{"command", "args", "stdin", "timeout_seconds", "timeout", "working_directory"}
	out := make(map[string]any)
	for _, k := range keys {
		if raw, ok := envelope[k]; ok {
			var v any
			if err := json.Unmarshal(raw, &v); err == nil {
				out[k] = v
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

func detectEmbeddedError(payload []byte) string {
	data := bytes.TrimSpace(payload)
	if len(data) == 0 {
		return ""
	}
	if bytes.Equal(data, []byte("null")) {
		return ""
	}
	if data[0] != '{' {
		return ""
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return ""
	}
	if okVal, ok := body["ok"]; ok {
		if okBool, ok := okVal.(bool); ok && !okBool {
			if msg, ok := extractString(body["error"]); ok {
				return msg
			}
			return "tool returned ok=false"
		}
	}
	if msg, ok := extractString(body["error"]); ok {
		return msg
	}
	return ""
}

func extractString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t), true
	case json.RawMessage:
		var s string
		if err := json.Unmarshal(t, &s); err == nil {
			return strings.TrimSpace(s), true
		}
	}
	return "", false
}
