package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	if len(toolCalls) == 0 {
		return msgs
	}

	maxParallel := e.MaxToolParallelism
	if maxParallel <= 0 || maxParallel > len(toolCalls) {
		maxParallel = len(toolCalls)
	}
	if maxParallel <= 0 {
		maxParallel = 1
	}

	results := make([]llm.Message, len(toolCalls))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, tc := range toolCalls {

		dispatchCtx := ctx
		if e.LLM != nil {
			dispatchCtx = tools.WithProvider(ctx, e.LLM)
		}
		dispatchCtx = tools.WithNestedToolDispatcher(dispatchCtx, func(childCtx context.Context, name string, raw json.RawMessage, toolCallID string) ([]byte, bool) {
			if !e.canHandleNestedDelegation(name) {
				return nil, false
			}
			id := strings.TrimSpace(toolCallID)
			if id == "" {
				id = e.nextToolCallID()
			}
			payload := e.runDelegatedTool(childCtx, llm.ToolCall{
				ID:   id,
				Name: name,
				Args: raw,
			})
			return payload, true
		})

		if tc.Name == "text_to_speech" && e.OnTool != nil {
			var raw map[string]any
			_ = json.Unmarshal(tc.Args, &raw)
			if v, ok := raw["stream"].(bool); ok && v {
				cb := func(chunk []byte) {
					meta := map[string]any{"event": "chunk", "bytes": len(chunk), "b64": base64.StdEncoding.EncodeToString(chunk)}
					b, _ := json.Marshal(meta)
					if e.OnTool != nil {
						e.OnTool("text_to_speech_chunk", tc.Args, b, tc.ID)
					}
				}
				dispatchCtx = tts.WithStreamChunkCallback(dispatchCtx, cb)
			}
		}

		if e.OnToolStart != nil {
			e.OnToolStart(tc.Name, tc.Args, tc.ID)
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(idx int, tc llm.ToolCall, dctx context.Context) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = e.executeToolCall(dctx, tc)
		}(i, tc, dispatchCtx)
	}

	wg.Wait()
	// Invoke OnTurnMessage for each tool response message
	if e.OnTurnMessage != nil {
		for _, toolMsg := range results {
			e.OnTurnMessage(toolMsg)
		}
	}
	return append(msgs, results...)
}

func (e *Engine) executeToolCall(ctx context.Context, tc llm.ToolCall) llm.Message {
	decision := e.evaluateToolPolicy(ctx, tc)
	if !decision.Allowed {
		payload, _ := json.Marshal(map[string]any{"ok": false, "error": "tool call blocked by policy", "policy_id": decision.RecordID, "message": decision.Message})
		if e.OnTool != nil {
			e.OnTool(tc.Name, tc.Args, payload, tc.ID)
		}
		return llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID}
	}
	if len(decision.Annotations) > 0 {
		observability.LoggerWithTrace(ctx).Info().Str("tool", tc.Name).Strs("policy_annotations", decision.Annotations).Strs("policy_ids", decision.MatchedIDs).Msg("policy_tool_call_annotated")
	}

	// Handle delegation as a first-class engine feature (not a tool).
	if e.canHandleNestedDelegation(tc.Name) {
		payload := e.runDelegatedTool(ctx, tc)
		if e.OnTool != nil {
			e.OnTool(tc.Name, tc.Args, payload, tc.ID)
		}
		return llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID}
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
		payload = fmt.Appendf(nil, `{"error":%q}`, err.Error())
	}
	if e.OnTool != nil {
		e.OnTool(tc.Name, tc.Args, payload, tc.ID)
	}
	return llm.Message{Role: "tool", Content: string(payload), ToolID: tc.ID}
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
		return fmt.Appendf(nil, `{"ok":false,"agent":%q,"error":%q}`, req.AgentName, err.Error())
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
		return fmt.Appendf(nil, `{"ok":false,"team":%q,"error":%q}`, req.TeamName, err.Error())
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
