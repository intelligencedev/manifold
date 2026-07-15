package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
)

// AgentCallTool invokes a named specialist (as a full agent) or the default agent engine
// with the provided prompt. Unlike specialists_infer (which is single-shot), this tool
// runs a full multi-step agent loop (with tools if enabled).
type AgentCallTool struct {
	// Optional default registry and specialists registry; the per-request registry can be
	// overridden via specialists_tool.WithRegistry on the context in HTTP handlers.
	reg        tools.Registry
	specReg    *specialists.Registry
	wsMgr      workspaces.WorkspaceManager
	defaultSys string
	// Default max steps if not provided in the call. A value of 0 is unbounded.
	defaultMaxSteps int
	// defaultTimeout, if > 0, is applied when the parent context has no deadline
	// and the caller does not provide timeout_seconds.
	defaultTimeout time.Duration
	// lexMinify* copy server-wide provider minification settings into nested engines.
	lexMinifyLevel      int
	lexMinifyZones      int
	lexMinifyCurrentMax int
}

type agentCallArgs struct {
	AgentName      string        `json:"agent_name"`
	Prompt         string        `json:"prompt"`
	History        []llm.Message `json:"history"`
	EnableTools    *bool         `json:"enable_tools"`
	MaxSteps       int           `json:"max_steps"`
	TimeoutSeconds int           `json:"timeout_seconds"`
	ProjectID      string        `json:"project_id"`
	UserID         int64         `json:"user_id"`
}

func NewAgentCallTool(reg tools.Registry, specReg *specialists.Registry, wsMgr workspaces.WorkspaceManager) *AgentCallTool {
	return &AgentCallTool{reg: reg, specReg: specReg, wsMgr: wsMgr, defaultSys: "You are a helpful assistant."}
}

func (t *AgentCallTool) SetDefaultMaxSteps(maxSteps int) {
	t.defaultMaxSteps = maxSteps
}

func (t *AgentCallTool) SetLexMinify(level, zones, currentMax int) {
	t.lexMinifyLevel = level
	t.lexMinifyZones = zones
	t.lexMinifyCurrentMax = currentMax
}

// SetDefaultTimeoutSeconds sets a default timeout applied when the parent context
// has no deadline and timeout_seconds is not specified by the caller. A value of
// 0 disables the default.
func (t *AgentCallTool) SetDefaultTimeoutSeconds(seconds int) {
	if seconds > 0 {
		t.defaultTimeout = time.Duration(seconds) * time.Second
	} else {
		t.defaultTimeout = 0
	}
}

func (t *AgentCallTool) Name() string { return "agent_call" }

func (t *AgentCallTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Invoke a named agent/specialist to run a multi-step reasoning loop (with tools).",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_name": map[string]any{
					"type":        "string",
					"description": "Optional specialist/agent name to invoke (see /api/specialists). If empty, uses the default agent.",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "User prompt to send to the agent.",
				},
				"history": map[string]any{
					"type":        "array",
					"description": "Optional prior messages as an array of {role, content}.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"role":    map[string]any{"type": "string"},
							"content": map[string]any{"type": "string"},
						},
						"required": []string{"role", "content"},
					},
				},
				"enable_tools": map[string]any{
					"type":        "boolean",
					"description": "If true, tools will be available to the invoked agent (subject to policy). Defaults to true for specialists that enable tools.",
				},
				"max_steps": map[string]any{
					"type":        "integer",
					"description": "Maximum reasoning steps for the agent loop. Defaults to configured maxSteps; 0 means unbounded.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout for the agent run in seconds.",
				},
				"project_id": map[string]any{
					"type":        "string",
					"description": "Optional project ID to scope the agent's sandbox (must match projects/<id> under workdir; not the display name).",
				},
				"user_id": map[string]any{
					"type":        "integer",
					"description": "Optional user ID (defaults to system user 0) used with project_id to build sandbox path.",
				},
			},
			"required": []string{"prompt"},
		},
	}
}

func (t *AgentCallTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args agentCallArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	dispatchCtx, response, err := t.dispatchContext(ctx, args)
	if err != nil || response != nil {
		return response, err
	}
	if response := t.callSpecialist(ctx, dispatchCtx, args); response != nil {
		return response, nil
	}
	return t.callLocalAgent(ctx, dispatchCtx, args)
}

func (t *AgentCallTool) dispatchContext(ctx context.Context, args agentCallArgs) (context.Context, any, error) {
	pid := strings.TrimSpace(args.ProjectID)
	if pid == "" || t.wsMgr == nil {
		return ctx, nil, nil
	}
	ws, err := t.wsMgr.Checkout(ctx, args.UserID, pid, "")
	if err != nil {
		return ctx, workspaceCheckoutError(err), nil
	}
	if ws.BaseDir == "" {
		return ctx, nil, nil
	}
	return sandbox.WithBaseDir(ctx, ws.BaseDir), nil, nil
}

func workspaceCheckoutError(err error) map[string]any {
	if err == workspaces.ErrInvalidProjectID {
		return map[string]any{"ok": false, "error": "invalid project_id"}
	}
	if err == workspaces.ErrProjectNotFound {
		return map[string]any{"ok": false, "error": "project not found (project_id must match the project directory/ID)"}
	}
	return map[string]any{"ok": false, "error": fmt.Sprintf("workspace checkout failed: %v", err)}
}

func (t *AgentCallTool) callSpecialist(ctx, dispatchCtx context.Context, args agentCallArgs) map[string]any {
	name := args.AgentName
	if name == "" || t.specReg == nil {
		return nil
	}
	a, ok := t.specReg.Get(name)
	if !ok || a == nil {
		return nil
	}
	observability.LoggerWithTrace(ctx).Info().Str("agent_call", name).Msg("agent_call_specialist_infer")
	out, err := a.Inference(dispatchCtx, args.Prompt, args.History)
	if err != nil {
		return map[string]any{"ok": false, "agent": name, "error": err.Error()}
	}
	return map[string]any{"ok": true, "agent": name, "output": out}
}

func (t *AgentCallTool) callLocalAgent(ctx, dispatchCtx context.Context, args agentCallArgs) (any, error) {
	prov := tools.ProviderFromContext(ctx)
	if prov == nil {
		return map[string]any{"ok": false, "error": "no llm provider available for agent_call"}, nil
	}
	eng := t.newEngine(prov, args)
	runCtx, cancel := t.runContext(ctx, dispatchCtx, args.TimeoutSeconds)
	if cancel != nil {
		defer cancel()
	}
	observability.LoggerWithTrace(ctx).Info().Str("agent_call", args.AgentName).Msg("agent_call_start")
	out, err := eng.Run(runCtx, args.Prompt, args.History)
	if err != nil {
		observability.LoggerWithTrace(ctx).Error().Err(err).Str("agent_call", args.AgentName).Msg("agent_call_error")
		payload := delegatedRunErrorPayload(err)
		payload["agent"] = args.AgentName
		return payload, nil
	}
	return map[string]any{"ok": true, "agent": args.AgentName, "output": out}, nil
}

func (t *AgentCallTool) newEngine(prov llm.Provider, args agentCallArgs) *agent.Engine {
	maxSteps := args.MaxSteps
	if maxSteps <= 0 {
		maxSteps = t.defaultMaxSteps
	}
	toolsReg := t.reg
	if args.EnableTools != nil && !*args.EnableTools {
		toolsReg = tools.NewRegistry()
	} else if toolsReg == nil {
		toolsReg = tools.NewRegistry()
	}
	eng := &agent.Engine{LLM: prov, Tools: toolsReg, MaxSteps: maxSteps, System: prompts.EnsureMemoryInstructions(t.defaultSys), LexMinifyLevel: t.lexMinifyLevel, LexMinifyZones: t.lexMinifyZones, LexMinifyCurrentMax: t.lexMinifyCurrentMax}
	eng.AttachTokenizer(prov, nil)
	return eng
}

func (t *AgentCallTool) runContext(ctx, dispatchCtx context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds > 0 {
		return context.WithTimeout(dispatchCtx, time.Duration(timeoutSeconds)*time.Second)
	}
	if _, has := ctx.Deadline(); !has && t.defaultTimeout > 0 {
		return context.WithTimeout(dispatchCtx, t.defaultTimeout)
	}
	return dispatchCtx, nil
}
