package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"manifold/internal/agent/inputrequest"
	"manifold/internal/durable"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/policy"
	"manifold/internal/tools"
	"manifold/internal/tools/tts"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func (e *Engine) ensureToolCallIDs(msgs []llm.Message, toolCalls []llm.ToolCall) []llm.ToolCall {
	used := make(map[string]struct{}, len(toolCalls))
	for _, msg := range msgs {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if id := strings.TrimSpace(tc.ID); id != "" {
				used[id] = struct{}{}
			}
		}
	}
	for i := range toolCalls {
		id := strings.TrimSpace(toolCalls[i].ID)
		hasSig := strings.TrimSpace(toolCalls[i].ThoughtSignature) != ""
		if id == "" {
			id = e.nextToolCallID()
		}
		if !hasSig {
			if _, ok := used[id]; ok {
				id = e.nextToolCallID()
			}
			for {
				if _, ok := used[id]; !ok {
					break
				}
				id = e.nextToolCallID()
			}
		}
		toolCalls[i].ID = id
		used[id] = struct{}{}
	}
	return toolCalls
}

func (e *Engine) nextToolCallID() string {
	seq := atomic.AddUint64(&e.toolCallSeq, 1)
	return fmt.Sprintf("engine-call-%d", seq)
}

// dispatchTools executes a batch of tool calls, appending their tool messages to msgs
// and invoking the appropriate callbacks/logging. It returns the updated msgs slice.
func (e *Engine) dispatchTools(ctx context.Context, msgs []llm.Message, toolCalls []llm.ToolCall) []llm.Message {
	msgs, _ = e.dispatchToolsAtStep(ctx, msgs, toolCalls, -1)
	return msgs
}

func (e *Engine) dispatchToolsAtStep(ctx context.Context, msgs []llm.Message, toolCalls []llm.ToolCall, step int) ([]llm.Message, error) {
	if len(toolCalls) == 0 {
		return msgs, nil
	}

	results := make([]llm.Message, len(toolCalls))
	sem := make(chan struct{}, e.toolDispatchParallelism(len(toolCalls)))
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for i, tc := range toolCalls {
		if checkpoint, ok := e.checkpointedToolResult(ctx, step, tc); ok {
			results[i] = checkpoint
			continue
		}

		if e.OnToolStartWithTitle != nil {
			e.OnToolStartWithTitle(tc.Name, e.toolTitle(tc.Name), tc.Args, tc.ID)
		} else if e.OnToolStart != nil {
			e.OnToolStart(tc.Name, tc.Args, tc.ID)
		}

		dispatchCtx := e.toolDispatchContext(ctx, tc)
		sem <- struct{}{}
		wg.Add(1)
		go func(idx int, tc llm.ToolCall, dctx context.Context) {
			defer wg.Done()
			defer func() { <-sem }()
			toolMsg, err := e.executeToolCallForStep(dctx, tc, step)
			if errors.Is(err, durable.ErrSuspended) {
				recordFirstToolDispatchError(&errMu, &firstErr, err)
				return
			}
			results[idx] = toolMsg
		}(i, tc, dispatchCtx)
	}

	wg.Wait()
	if firstErr != nil {
		return msgs, firstErr
	}
	// Invoke OnTurnMessage for each tool response message
	if e.OnTurnMessage != nil {
		for _, toolMsg := range results {
			e.OnTurnMessage(toolMsg)
		}
	}
	return append(msgs, results...), nil
}

func (e *Engine) toolDispatchParallelism(toolCount int) int {
	maxParallel := e.MaxToolParallelism
	if maxParallel <= 0 || maxParallel > toolCount {
		maxParallel = toolCount
	}
	if maxParallel <= 0 {
		return 1
	}
	return maxParallel
}

func (e *Engine) checkpointedToolResult(ctx context.Context, step int, tc llm.ToolCall) (llm.Message, bool) {
	if step < 0 {
		return llm.Message{}, false
	}
	var checkpoint llm.Message
	found, err := e.loadCheckpoint(ctx, toolCheckpointKey(step, tc.ID), &checkpoint)
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Str("tool", tc.Name).Str("tool_id", tc.ID).Int("step", step).Msg("engine_tool_checkpoint_load_failed")
		return llm.Message{}, false
	}
	return checkpoint, found
}

func (e *Engine) toolDispatchContext(ctx context.Context, tc llm.ToolCall) context.Context {
	dispatchCtx := ctx
	if e.LLM != nil {
		dispatchCtx = tools.WithProvider(ctx, e.LLM)
	}
	dispatchCtx = tools.WithNestedToolDispatcher(dispatchCtx, e.nestedToolDispatcher())
	return e.withStreamingTTSCallback(dispatchCtx, tc)
}

func (e *Engine) nestedToolDispatcher() tools.NestedToolDispatcher {
	return func(childCtx context.Context, name string, raw json.RawMessage, toolCallID string) ([]byte, bool) {
		if !e.canHandleNestedDelegation(name) {
			return nil, false
		}
		id := strings.TrimSpace(toolCallID)
		if id == "" {
			id = e.nextToolCallID()
		}
		payload := e.runDelegatedTool(childCtx, llm.ToolCall{ID: id, Name: name, Args: raw})
		return payload, true
	}
}

func (e *Engine) withStreamingTTSCallback(ctx context.Context, tc llm.ToolCall) context.Context {
	if tc.Name != "text_to_speech" || e.OnTool == nil {
		return ctx
	}
	var raw map[string]any
	if err := json.Unmarshal(tc.Args, &raw); err != nil {
		return ctx
	}
	if stream, ok := raw["stream"].(bool); !ok || !stream {
		return ctx
	}
	cb := func(chunk []byte) {
		meta := map[string]any{"event": "chunk", "bytes": len(chunk), "b64": base64.StdEncoding.EncodeToString(chunk)}
		b, _ := json.Marshal(meta)
		if e.OnTool != nil {
			e.OnTool("text_to_speech_chunk", tc.Args, b, tc.ID)
		}
	}
	return tts.WithStreamChunkCallback(ctx, cb)
}

func (e *Engine) executeToolCallForStep(ctx context.Context, tc llm.ToolCall, step int) (llm.Message, error) {
	payload, err := e.executeToolCallPayload(ctx, tc)
	if err != nil {
		if errors.Is(err, durable.ErrSuspended) {
			return llm.Message{}, err
		}
		payload = fmt.Appendf(nil, `{"error":%q}`, err.Error())
	}
	toolMsg := llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID}
	if step >= 0 {
		if err := e.saveCheckpoint(ctx, toolCheckpointKey(step, tc.ID), toolMsg); err != nil {
			observability.LoggerWithTrace(ctx).Warn().Err(err).Str("tool", tc.Name).Str("tool_id", tc.ID).Int("step", step).Msg("engine_tool_checkpoint_save_failed")
		}
	}
	e.emitToolResult(tc.Name, tc.Args, payload, tc.ID)
	return toolMsg, nil
}

func recordFirstToolDispatchError(mu *sync.Mutex, target *error, err error) {
	mu.Lock()
	defer mu.Unlock()
	if *target == nil {
		*target = err
	}
}

func (e *Engine) executeToolCall(ctx context.Context, tc llm.ToolCall) llm.Message {
	payload, err := e.executeToolCallPayload(ctx, tc)
	if err != nil {
		payload = fmt.Appendf(nil, `{"error":%q}`, err.Error())
	}
	e.emitToolResult(tc.Name, tc.Args, payload, tc.ID)
	return llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID}
}

func (e *Engine) executeToolCallPayload(ctx context.Context, tc llm.ToolCall) ([]byte, error) {
	meta := inputrequest.RunMetadataFromContext(ctx)
	if strings.TrimSpace(meta.ToolID) == "" && strings.TrimSpace(tc.ID) != "" {
		meta.ToolID = strings.TrimSpace(tc.ID)
		ctx = inputrequest.WithRunMetadata(ctx, meta)
	}
	decision := e.evaluateToolPolicy(ctx, tc)
	if !decision.Allowed {
		payload, _ := json.Marshal(map[string]any{"ok": false, "error": "tool call blocked by policy", "policy_id": decision.RecordID, "message": decision.Message})
		return payload, nil
	}
	if len(decision.Annotations) > 0 {
		observability.LoggerWithTrace(ctx).Info().Str("tool", tc.Name).Strs("policy_annotations", decision.Annotations).Strs("policy_ids", decision.MatchedIDs).Msg("policy_tool_call_annotated")
	}

	// Handle delegation as a first-class engine feature (not a tool).
	if e.canHandleNestedDelegation(tc.Name) {
		payload := e.runDelegatedTool(ctx, tc)
		return payload, nil
	}

	redactedArgs := observability.RedactJSON(tc.Args)
	event := observability.LoggerWithTrace(ctx).Info().Str("tool", tc.Name)
	if json.Valid(redactedArgs) {
		event = event.RawJSON("args", redactedArgs)
	} else {
		event = event.Str("args", string(redactedArgs)).Bool("args_json_valid", false)
	}
	event.Msg("engine_tool_call")
	payload, err := e.Tools.Dispatch(ctx, tc.Name, tc.Args)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (e *Engine) evaluateToolPolicy(ctx context.Context, tc llm.ToolCall) policy.Decision {
	if e == nil || e.PolicyEnforcer == nil {
		return policy.Decision{Allowed: true}
	}
	decision, err := e.PolicyEnforcer.Evaluate(ctx, policy.EvaluationRequest{
		TenantID:    e.UserID,
		UserID:      e.UserID,
		ProjectID:   e.ProjectID,
		ObjectiveID: e.ObjectiveID,
		Role:        e.AgentRole,
		ToolName:    tc.Name,
		Args:        tc.Args,
	})
	if err != nil {
		observability.LoggerWithTrace(ctx).Warn().Err(err).Str("tool", tc.Name).Msg("policy_tool_call_evaluation_failed")
		return policy.Decision{Allowed: true}
	}
	if !decision.Allowed {
		observability.LoggerWithTrace(ctx).Warn().Str("tool", tc.Name).Str("policy_id", decision.RecordID).Str("message", decision.Message).Msg("policy_tool_call_blocked")
	}
	return decision
}

func isAgentCall(name string) bool {
	return name == "agent_call" || name == "ask_agent"
}

func isTeamCall(name string) bool {
	return name == "delegate_to_team"
}

func (e *Engine) canHandleNestedDelegation(name string) bool {
	if e == nil {
		return false
	}
	if isAgentCall(name) {
		return e.Delegator != nil
	}
	if isTeamCall(name) {
		return e.TeamDelegator != nil
	}
	return false
}

func (e *Engine) runDelegatedTool(ctx context.Context, tc llm.ToolCall) []byte {
	if isTeamCall(tc.Name) {
		return e.runDelegatedTeam(ctx, tc)
	}
	return e.runDelegatedAgent(ctx, tc)
}

// runDelegatedAgent executes an agent-to-agent handoff using the configured
// Delegator and wraps the output in the legacy tool payload shape so the
// parent loop can continue unchanged.
func (e *Engine) runDelegatedAgent(ctx context.Context, tc llm.ToolCall) []byte {
	var args struct {
		AgentName      string        `json:"agent_name"`
		To             string        `json:"to"`
		Prompt         string        `json:"prompt"`
		History        []llm.Message `json:"history"`
		EnableTools    *bool         `json:"enable_tools"`
		MaxSteps       int           `json:"max_steps"`
		TimeoutSeconds int           `json:"timeout_seconds"`
		ProjectID      string        `json:"project_id"`
		UserID         int64         `json:"user_id"`
	}
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return fmt.Appendf(nil, `{"ok":false,"error":%q}`, err.Error())
	}
	// Support both `agent_name` (internal) and `to` (ask_agent tool)
	if strings.TrimSpace(args.AgentName) == "" && strings.TrimSpace(args.To) != "" {
		args.AgentName = strings.TrimSpace(args.To)
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return []byte(`{"ok":false,"error":"prompt is required"}`)
	}
	projectID := strings.TrimSpace(args.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(e.ProjectID)
	}
	userID := args.UserID
	if userID == 0 {
		userID = e.UserID
	}
	callID := tc.ID
	if strings.TrimSpace(callID) == "" {
		callID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	req := DelegateRequest{
		AgentName:             strings.TrimSpace(args.AgentName),
		Prompt:                args.Prompt,
		History:               args.History,
		EnableTools:           args.EnableTools,
		MaxSteps:              args.MaxSteps,
		TimeoutSeconds:        args.TimeoutSeconds,
		ProjectID:             projectID,
		ObjectiveID:           strings.TrimSpace(e.ObjectiveID),
		SessionID:             e.SessionID,
		UserID:                userID,
		CallID:                callID,
		ParentCallID:          tc.ID,
		Depth:                 e.AgentDepth + 1,
		DisableEvolvingMemory: e.DisableEvolvingMemory,
		DisableBeliefMemory:   e.DisableBeliefMemory,
	}
	result, err := e.Delegator.Run(ctx, req, e.AgentTracer)
	if err != nil {
		return delegatedRunErrorJSON("agent", req.AgentName, err)
	}
	out := map[string]any{"ok": true, "agent": req.AgentName, "output": result}
	if b, err := json.Marshal(out); err == nil {
		return b
	}
	return []byte(result)
}

// runDelegatedTeam executes a team handoff using the configured TeamDelegator
// and wraps the output in the legacy delegate_to_team payload shape so the
// parent loop can continue unchanged.
func (e *Engine) runDelegatedTeam(ctx context.Context, tc llm.ToolCall) []byte {
	var args struct {
		Team           string        `json:"team"`
		Prompt         string        `json:"prompt"`
		History        []llm.Message `json:"history"`
		TimeoutMS      int           `json:"timeout_ms"`
		TimeoutSeconds int           `json:"timeout_seconds"`
		ProjectID      string        `json:"project_id"`
		ObjectiveID    string        `json:"objective_id"`
		SessionID      string        `json:"session_id"`
		UserID         int64         `json:"user_id"`
	}
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return fmt.Appendf(nil, `{"ok":false,"error":%q}`, err.Error())
	}
	teamName := strings.TrimSpace(args.Team)
	if teamName == "" {
		return []byte(`{"ok":false,"error":"team is required"}`)
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return []byte(`{"ok":false,"error":"prompt is required"}`)
	}
	projectID := strings.TrimSpace(args.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(e.ProjectID)
	}
	objectiveID := strings.TrimSpace(args.ObjectiveID)
	if objectiveID == "" {
		objectiveID = strings.TrimSpace(e.ObjectiveID)
	}
	sessionID := strings.TrimSpace(args.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(e.SessionID)
	}
	userID := args.UserID
	if userID == 0 {
		userID = e.UserID
	}
	callID := strings.TrimSpace(tc.ID)
	if callID == "" {
		callID = fmt.Sprintf("team-%d", time.Now().UnixNano())
	}
	req := TeamDelegateRequest{
		TeamName:              teamName,
		Prompt:                args.Prompt,
		History:               args.History,
		TimeoutSeconds:        args.TimeoutSeconds,
		TimeoutMS:             args.TimeoutMS,
		ProjectID:             projectID,
		ObjectiveID:           objectiveID,
		SessionID:             sessionID,
		UserID:                userID,
		CallID:                callID,
		ParentCallID:          tc.ID,
		Depth:                 e.AgentDepth + 1,
		DisableEvolvingMemory: e.DisableEvolvingMemory,
		DisableBeliefMemory:   e.DisableBeliefMemory,
	}
	result, err := e.TeamDelegator.RunTeam(ctx, req, e.AgentTracer)
	if err != nil {
		return delegatedRunErrorJSON("team", req.TeamName, err)
	}
	out := map[string]any{
		"ok":   true,
		"team": req.TeamName,
		"response": map[string]any{
			"result": result,
		},
	}
	if b, err := json.Marshal(out); err == nil {
		return b
	}
	return []byte(result)
}

func delegatedRunErrorJSON(kind, name string, err error) []byte {
	payload := map[string]any{
		"ok":    false,
		kind:    name,
		"error": err.Error(),
	}
	switch {
	case errors.Is(err, context.Canceled):
		payload["error"] = "delegated run cancelled"
		payload["error_code"] = "delegated_run_cancelled"
		payload["cancelled"] = true
	case errors.Is(err, context.DeadlineExceeded):
		payload["error_code"] = "delegated_run_timeout"
		payload["timed_out"] = true
	case errors.Is(err, ErrMaxStepsExceeded):
		payload["error_code"] = "delegated_run_max_steps"
		payload["max_steps_exceeded"] = true
	}
	if b, marshalErr := json.Marshal(payload); marshalErr == nil {
		return b
	}
	return fmt.Appendf(nil, `{"ok":false,%q:%q,"error":%q}`, kind, name, payload["error"])
}

func (e *Engine) toolTitle(name string) string {
	return tools.ToolTitle(e.Tools, name)
}

func (e *Engine) emitToolResult(name string, args []byte, result []byte, toolID string) {
	if e.OnToolWithTitle != nil {
		e.OnToolWithTitle(name, e.toolTitle(name), args, result, toolID)
		return
	}
	if e.OnTool != nil {
		e.OnTool(name, args, result, toolID)
	}
}
