package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/tools"
)

// Delegator bridges agent-to-agent calls directly through the agent engine
// rather than the tool registry. It supports tracing nested interactions so
// UIs can render sub-agent activity.
type Delegator struct {
	reg            tools.Registry
	specReg        *specialists.Registry
	workdir        string
	defaultSys     string
	defaultMaxStep int
	defaultTimeout time.Duration
}

func NewDelegator(reg tools.Registry, specReg *specialists.Registry, workdir string, defaultMaxSteps int) *Delegator {
	return &Delegator{reg: reg, specReg: specReg, workdir: workdir, defaultSys: "You are a helpful assistant.", defaultMaxStep: defaultMaxSteps}
}

func (d *Delegator) SetDefaultTimeout(seconds int) {
	if seconds > 0 {
		d.defaultTimeout = time.Duration(seconds) * time.Second
	}
}

// SetRegistry updates the internal tools registry used by delegated agent runs.
// This allows the orchestrator to rebuild its tool registry (e.g., allowlists)
// and propagate the change to the delegator without recreating it.
func (d *Delegator) SetRegistry(reg tools.Registry) {
	d.reg = reg
}

func (d *Delegator) Run(ctx context.Context, req agent.DelegateRequest, tracer agent.AgentTracer) (string, error) {
	dispatchCtx := ctx
	if pid := req.ProjectID; pid != "" && d.workdir != "" {
		base := filepath.Join(d.workdir, "users", fmt.Sprint(req.UserID), "projects", pid)
		if st, err := os.Stat(base); err != nil || !st.IsDir() {
			return "", fmt.Errorf("project not found (project_id must match the project directory/ID)")
		}
		dispatchCtx = sandbox.WithBaseDir(dispatchCtx, base)
	}

	var prov llm.Provider
	var toolsReg tools.Registry
	system := d.defaultSys
	model := ""

	toolsReg = d.reg

	if req.AgentName != "" && d.specReg != nil {
		if a, ok := d.specReg.Get(req.AgentName); ok && a != nil {
			prov = a.Provider()
			toolsReg = a.ToolsRegistry()
			// The specialist's System field already has the default prompt prepended
			// during registry initialization, so use it directly
			system = a.System
			model = a.Model
			if a.EnableTools && toolsReg == nil {
				toolsReg = tools.NewRegistry()
			}
		}
	}
	if prov == nil {
		if p := tools.ProviderFromContext(dispatchCtx); p != nil {
			prov = p
		}
	}
	if prov == nil {
		return "", fmt.Errorf("no llm provider available for delegated agent")
	}

	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = d.defaultMaxStep
		if maxSteps <= 0 {
			maxSteps = 8
		}
	}
	if req.EnableTools != nil && !*req.EnableTools {
		toolsReg = tools.NewRegistry()
	} else if toolsReg == nil {
		toolsReg = tools.NewRegistry()
	}

	runCtx := dispatchCtx
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(dispatchCtx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	} else if _, has := dispatchCtx.Deadline(); !has && d.defaultTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(dispatchCtx, d.defaultTimeout)
		defer cancel()
	}

	if tracer != nil {
		tracer.Trace(agent.AgentTrace{Type: "agent_start", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: req.Prompt})
	}

	eng := &agent.Engine{
		LLM:         prov,
		Tools:       toolsReg,
		MaxSteps:    maxSteps,
		System:      prompts.EnsureMemoryInstructions(system),
		Model:       model,
		Delegator:   d,
		AgentTracer: tracer,
		AgentDepth:  req.Depth,
	}

	if tracer != nil {
		eng.OnDelta = func(delta string) {
			if delta == "" {
				return
			}
			tracer.Trace(agent.AgentTrace{Type: "agent_delta", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: delta, Role: "assistant"})
		}
		eng.OnToolStart = func(name string, args []byte, toolID string) {
			tracer.Trace(agent.AgentTrace{Type: "agent_tool_start", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Title: name, Args: string(args), ToolID: toolID})
		}
		eng.OnTool = func(name string, args []byte, result []byte, toolID string) {
			tracer.Trace(agent.AgentTrace{Type: "agent_tool_result", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Title: name, Args: string(args), Data: string(result), ToolID: toolID})
		}
	}

	observability.LoggerWithTrace(ctx).Info().Str("agent_delegate", req.AgentName).Msg("delegated_agent_start")
	out, err := eng.Run(runCtx, req.Prompt, req.History)
	if err != nil {
		if tracer != nil {
			tracer.Trace(agent.AgentTrace{Type: "agent_error", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Error: err.Error()})
		}
		return "", err
	}
	if tracer != nil {
		tracer.Trace(agent.AgentTrace{Type: "agent_final", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: out})
	}
	return out, nil
}
