package agents

import (
	"context"
	"encoding/json"
	"time"

	"manifold/internal/agent"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/specialists"
	"manifold/internal/tools"
)

// AgentCallTool invokes a named specialist (as a full agent) or the default agent engine
// with the provided prompt. Unlike specialists_infer (which is single-shot), this tool
// runs a full multi-step agent loop (with tools if enabled).
type AgentCallTool struct {
	// Optional default registry and specialists registry; the per-request registry can be
	// overridden via specialists_tool.WithRegistry on the context in HTTP handlers.
	reg        tools.Registry
	specReg    *specialists.Registry
	defaultSys string
	// Max default steps if not provided in the call
	defaultMaxSteps int
}

func NewAgentCallTool(reg tools.Registry, specReg *specialists.Registry) *AgentCallTool {
	return &AgentCallTool{reg: reg, specReg: specReg, defaultSys: "You are a helpful assistant.", defaultMaxSteps: 8}
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
					"description": "Maximum reasoning steps for the agent loop (default 8).",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout for the agent run in seconds.",
				},
			},
			"required": []string{"prompt"},
		},
	}
}

func (t *AgentCallTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		AgentName      string        `json:"agent_name"`
		Prompt         string        `json:"prompt"`
		History        []llm.Message `json:"history"`
		EnableTools    *bool         `json:"enable_tools"`
		MaxSteps       int           `json:"max_steps"`
		TimeoutSeconds int           `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	// Resolve provider and tool registry view
	var prov llm.Provider
	var toolsReg tools.Registry
	var system string

	toolsReg = t.reg
	system = t.defaultSys

	// Prefer provider from context when present
	if p := tools.ProviderFromContext(ctx); p != nil {
		prov = p
	}

	// If a specialist/agent name is provided, use its configured provider and tools view
	if name := args.AgentName; name != "" && t.specReg != nil {
		if a, ok := t.specReg.Get(name); ok && a != nil {
			// Delegate to specialist single-shot inference for Phase 1 minimal implementation
			observability.LoggerWithTrace(ctx).Info().Str("agent_call", name).Msg("agent_call_specialist_infer")
			out, err := a.Inference(ctx, args.Prompt, args.History)
			if err != nil {
				return map[string]any{"ok": false, "agent": name, "error": err.Error()}, nil
			}
			return map[string]any{"ok": true, "agent": name, "output": out}, nil
		}
	}

	// Fallback to running a local agent engine with the current provider and tool registry
	if prov = tools.ProviderFromContext(ctx); prov == nil {
		return map[string]any{"ok": false, "error": "no llm provider available for agent_call"}, nil
	}
	maxSteps := args.MaxSteps
	if maxSteps <= 0 {
		maxSteps = t.defaultMaxSteps
	}
	if args.EnableTools != nil && !*args.EnableTools {
		toolsReg = tools.NewRegistry()
	} else if toolsReg == nil {
		toolsReg = tools.NewRegistry()
	}
	eng := &agent.Engine{LLM: prov, Tools: toolsReg, MaxSteps: maxSteps, System: system}
	runCtx := ctx
	if args.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(args.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	observability.LoggerWithTrace(ctx).Info().Str("agent_call", args.AgentName).Msg("agent_call_start")
	out, err := eng.Run(runCtx, args.Prompt, args.History)
	if err != nil {
		observability.LoggerWithTrace(ctx).Error().Err(err).Str("agent_call", args.AgentName).Msg("agent_call_error")
		return map[string]any{"ok": false, "agent": args.AgentName, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "agent": args.AgentName, "output": out}, nil
}
