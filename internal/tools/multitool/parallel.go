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

	"github.com/google/uuid"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/durable"
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
		"description": "Run multiple function tools concurrently when their work is independent. This is Manifold's multi_tool_use.parallel tool. Each entry in tool_uses MUST have shape {\"recipient_name\":\"functions.<tool>\",\"parameters\":{...tool args...}}. Example fanning out to two specialists via ask_agent: {\"tool_uses\":[{\"recipient_name\":\"functions.ask_agent\",\"parameters\":{\"to\":\"researcher\",\"prompt\":\"...\"}},{\"recipient_name\":\"functions.ask_agent\",\"parameters\":{\"to\":\"writer\",\"prompt\":\"...\"}}]}.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool_uses": map[string]any{
					"type":        "array",
					"description": "List of tool invocations to execute in parallel. Every item must nest tool arguments inside a parameters object.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"recipient_name": map[string]any{
								"type":        "string",
								"description": "Tool identifier, e.g. functions.ask_agent or functions.web_fetch.",
							},
							"parameters": map[string]any{
								"type":        "object",
								"description": "JSON arguments object for the tool. For ask_agent use {\"to\":\"<specialist>\",\"prompt\":\"...\"}; for agent_call use {\"agent_name\":\"<specialist>\",\"prompt\":\"...\"}.",
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
	exec, err := t.prepareExecution(ctx, args)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return exec.run(ctx)
}

type parallelExecution struct {
	toolName    string
	registry    tools.Registry
	calls       []preparedParallelCall
	timeout     time.Duration
	maxParallel int
}

type preparedParallelCall struct {
	spec       parallelCall
	toolName   string
	toolCallID string
}

func (t *ParallelTool) prepareExecution(ctx context.Context, args parallelArgs) (parallelExecution, error) {
	reg := tools.DispatchRegistryFromContext(ctx)
	if reg == nil {
		reg = t.registryView()
	}
	if reg == nil {
		return parallelExecution{}, errors.New("tool registry unavailable")
	}
	if len(args.ToolUses) == 0 {
		return parallelExecution{}, errors.New("tool_uses must contain at least one entry")
	}
	if len(args.ToolUses) > 32 {
		return parallelExecution{}, errors.New("tool_uses exceeds maximum of 32")
	}
	calls, err := t.prepareCalls(args.ToolUses)
	if err != nil {
		return parallelExecution{}, err
	}
	return parallelExecution{
		toolName:    t.Name(),
		registry:    reg,
		calls:       calls,
		timeout:     time.Duration(args.TimeoutMS) * time.Millisecond,
		maxParallel: t.effectiveMaxParallel(len(calls)),
	}, nil
}

func (t *ParallelTool) prepareCalls(calls []parallelCall) ([]preparedParallelCall, error) {
	prepared := make([]preparedParallelCall, 0, len(calls))
	for _, call := range calls {
		toolName, err := normalizeRecipient(call.RecipientName)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient_name (%s): %v", call.RecipientName, err)
		}
		if toolName == t.Name() {
			return nil, errors.New("recursive multi_tool_use_parallel invocation is not allowed")
		}
		toolCallID := call.ToolCallID
		if toolCallID == "" {
			toolCallID = uuid.NewString()
		}
		prepared = append(prepared, preparedParallelCall{spec: call, toolName: toolName, toolCallID: toolCallID})
	}
	return prepared, nil
}

func (t *ParallelTool) effectiveMaxParallel(callCount int) int {
	maxParallel := t.maxParallel
	if maxParallel <= 0 || maxParallel > callCount {
		return callCount
	}
	return maxParallel
}

func (e parallelExecution) run(ctx context.Context) (any, error) {
	results := make([]callResult, len(e.calls))
	var errs []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	sem := make(chan struct{}, e.maxParallel)

	for idx, call := range e.calls {
		wg.Add(1)
		go func(i int, call preparedParallelCall) {
			defer wg.Done()
			result, errText, err := e.executeOne(ctx, sem, call)
			results[i] = result
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			if errText != "" {
				mu.Lock()
				errs = append(errs, errText)
				mu.Unlock()
			}
		}(idx, call)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	ok := len(errs) == 0
	resp := map[string]any{"ok": ok, "results": results}
	if !ok {
		resp["error"] = strings.Join(errs, "; ")
	}
	return resp, nil
}

func (e parallelExecution) executeOne(ctx context.Context, sem chan struct{}, call preparedParallelCall) (callResult, string, error) {
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		errMsg := ctx.Err().Error()
		return callResult{
			RecipientName: call.spec.RecipientName,
			ToolName:      call.toolName,
			ToolCallID:    call.toolCallID,
			Error:         errMsg,
		}, fmt.Sprintf("%s: %s", call.toolName, errMsg), nil
result, err := durable.Step(ctx, parallelChildStepKey(ctx, call), func(stepCtx context.Context) (callResult, error) {
		dispatchCtx := stepCtx
		if e.timeout > 0 {
			var cancel context.CancelFunc
			dispatchCtx, cancel = context.WithTimeout(stepCtx, e.timeout)
			defer cancel()
		}
		dispatchCtx = withNestedToolMetadata(dispatchCtx, call.toolCallID)
		start := time.Now()
		payload, err := e.dispatch(dispatchCtx, call)
		out := callResult{
			RecipientName: call.spec.RecipientName,
			ToolName:      call.toolName,
			ToolCallID:    call.toolCallID,
			DurationMS:    time.Since(start).Milliseconds(),
		}
		if err != nil {
			if errors.Is(err, durable.ErrSuspended) {
				return callResult{}, err
			}
			out.Error = err.Error()
			return out, nil
		}
		return resultWithPayload(out, payload)
	})
	if err != nil {
		return callResult{}, "", err
	}
	if result.Error != "" {
		return result, fmt.Sprintf("%s: %s", result.ToolName, result.Error), nil
	}
	return result, "", nil
}

func (e parallelExecution) dispatch(ctx context.Context, call preparedParallelCall) ([]byte, error) {
	argsPayload := call.spec.Parameters
	if len(argsPayload) == 0 || string(argsPayload) == "null" {
		argsPayload = []byte("{}")
	}
	if dispatcher := tools.NestedToolDispatcherFromContext(ctx); dispatcher != nil {
		if payload, handled := dispatcher(ctx, call.toolName, argsPayload, call.toolCallID); handled {
			return payload, nil
		}
	}
	return e.registry.Dispatch(ctx, call.toolName, argsPayload)
}

func withNestedToolMetadata(ctx context.Context, toolCallID string) context.Context {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return ctx
	}
	meta := inputrequest.RunMetadataFromContext(ctx)
	meta.ToolID = toolCallID
	return inputrequest.WithRunMetadata(ctx, meta)
}

func parallelChildStepKey(ctx context.Context, call preparedParallelCall) string {
	id := strings.TrimSpace(call.toolCallID)
	if id == "" {
		id = strings.TrimSpace(call.toolName)
	}
	parent := strings.TrimSpace(inputrequest.RunMetadataFromContext(ctx).ToolID)
	if parent == "" {
		return "multi_tool_use_parallel:" + id
	}
	return "multi_tool_use_parallel:" + parent + ":" + id
}

func resultWithPayload(result callResult, payload []byte) (callResult, error) {
	if len(payload) == 0 {
		payload = []byte("null")
	}
	if embeddedErr := detectEmbeddedError(payload); embeddedErr != "" {
		result.Error = embeddedErr
		return result, nil
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	result.Payload = json.RawMessage(cp)
	return result, nil
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
	v = strings.TrimPrefix(v, "functions.")
	if v == "multi_tool_use.parallel" {
		v = ToolName
	}
	if strings.TrimSpace(v) == "" {
		return "", errors.New("recipient_name missing tool identifier")
	}
	return v, nil
}
	if v == "" {
		return "", errors.New("recipient_name is empty")
	}
	v = strings.TrimPrefix(v, "functions.")
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
		if params := liftSiblingParameters(envelope); params != nil {
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

// reservedEnvelopeKeys are keys consumed by the tool-use envelope itself; any
// other top-level key is treated as a tool argument that the model forgot to
// nest under "parameters".
var reservedEnvelopeKeys = map[string]struct{}{
	"recipient_name": {},
	"name":           {},
	"tool":           {},
	"tool_name":      {},
	"parameters":     {},
	"arguments":      {},
	"tool_call_id":   {},
	"id":             {},
}

// liftSiblingParameters synthesizes a parameters object from any non-envelope
// keys present in the call. This makes the parser tolerant of weaker models
// that emit flat shapes like {"recipient_name":"ask_agent","to":"x","prompt":"y"}
// instead of the schema-correct nested form.
func liftSiblingParameters(envelope map[string]json.RawMessage) json.RawMessage {
	out := make(map[string]any, len(envelope))
	for k, raw := range envelope {
		if _, reserved := reservedEnvelopeKeys[k]; reserved {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			continue
		}
		out[k] = v
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
