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
	"os"
	"path/filepath"
)

// AgentCallTool invokes a named specialist (as a full agent) or the default agent engine
// with the provided prompt. Unlike specialists_infer (which is single-shot), this tool
// runs a full multi-step agent loop (with tools if enabled).
type AgentCallTool struct {
	// Optional default registry and specialists registry; the per-request registry can be
	// overridden via specialists_tool.WithRegistry on the context in HTTP handlers.
	reg        tools.Registry
	specReg    *specialists.Registry
	workdir    string
	defaultSys string
	// Max default steps if not provided in the call
	defaultMaxSteps int
	// defaultTimeout, if > 0, is applied when the parent context has no deadline
	// and the caller does not provide timeout_seconds.
	defaultTimeout time.Duration
}

func NewAgentCallTool(reg tools.Registry, specReg *specialists.Registry, workdir string) *AgentCallTool {
	return &AgentCallTool{reg: reg, specReg: specReg, workdir: workdir, defaultSys: "You are a helpful assistant.", defaultMaxSteps: 8}
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
					"description": "Maximum reasoning steps for the agent loop (default 8).",
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
	var args struct {
		AgentName      string        `json:"agent_name"`
		Prompt         string        `json:"prompt"`
		History        []llm.Message `json:"history"`
		EnableTools    *bool         `json:"enable_tools"`
		MaxSteps       int           `json:"max_steps"`
		TimeoutSeconds int           `json:"timeout_seconds"`
		ProjectID      string        `json:"project_id"`
		UserID         int64         `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}

	dispatchCtx := ctx
	if pid := strings.TrimSpace(args.ProjectID); pid != "" && strings.TrimSpace(t.workdir) != "" {
		cleanPID := filepath.Clean(pid)
		if cleanPID != pid || strings.HasPrefix(cleanPID, "..") || strings.Contains(cleanPID, string(os.PathSeparator)+"..") || filepath.IsAbs(cleanPID) {
			return map[string]any{"ok": false, "error": "invalid project_id"}, nil
		}
		uid := args.UserID
		baseRoot := filepath.Join(t.workdir, "users", fmt.Sprint(uid), "projects")
		base := filepath.Join(baseRoot, cleanPID)
		if !strings.HasPrefix(base, baseRoot+string(os.PathSeparator)) && base != baseRoot {
			return map[string]any{"ok": false, "error": "invalid project_id"}, nil
		}
		if st, err := os.Stat(base); err != nil || !st.IsDir() {
			return map[string]any{"ok": false, "error": "project not found (project_id must match the project directory/ID)"}, nil
		}
		dispatchCtx = sandbox.WithBaseDir(ctx, base)
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
			// Note: The specialist's Inference method already handles system prompt composition
			// via buildMessages which includes its configured System field.
			// The System field should already have default instructions prepended during registry initialization.
			observability.LoggerWithTrace(ctx).Info().Str("agent_call", name).Msg("agent_call_specialist_infer")
			out, err := a.Inference(dispatchCtx, args.Prompt, args.History)
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
	eng := &agent.Engine{LLM: prov, Tools: toolsReg, MaxSteps: maxSteps, System: prompts.EnsureMemoryInstructions(system)}
	eng.AttachTokenizer(prov, nil)
	runCtx := ctx
	if args.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(dispatchCtx, time.Duration(args.TimeoutSeconds)*time.Second)
		defer cancel()
	} else if _, has := ctx.Deadline(); !has && t.defaultTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(dispatchCtx, t.defaultTimeout)
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
