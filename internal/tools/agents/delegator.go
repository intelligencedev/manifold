package agents

import (
	"context"
	"fmt"
	"time"

	"manifold/internal/agent"
	"manifold/internal/agent/belief"
	"manifold/internal/agent/inputrequest"
	"manifold/internal/agent/memory"
	"manifold/internal/agent/prompts"
	"manifold/internal/llm"
	"manifold/internal/observability"
	"manifold/internal/policy"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	inputrequesttool "manifold/internal/tools/inputrequest"
	"manifold/internal/workspaces"
)

// Delegator bridges agent-to-agent calls directly through the agent engine
// rather than the tool registry. It supports tracing nested interactions so
// UIs can render sub-agent activity.
type Delegator struct {
	reg             tools.Registry
	specReg         *specialists.Registry
	wsMgr           workspaces.WorkspaceManager
	defaultSys      string
	defaultMaxStep  int
	defaultTimeout  time.Duration
	evolvingMemory  *memory.EvolvingMemory
	reMemLLM        llm.Provider
	reMemModel      string
	reMemMaxSteps   int
	beliefStore     belief.Store
	beliefDistiller belief.Distiller
	beliefRetriever belief.Retriever
	beliefGraph     belief.Graph
	beliefMaxItems  int
	beliefMaxTokens int
	beliefThreshold float64
	policyEnforcer  policy.Enforcer
}

const defaultImagePromptSize = "1K"

func NewDelegator(reg tools.Registry, specReg *specialists.Registry, wsMgr workspaces.WorkspaceManager, defaultMaxSteps int) *Delegator {
	return &Delegator{reg: reg, specReg: specReg, wsMgr: wsMgr, defaultSys: "You are a helpful assistant.", defaultMaxStep: defaultMaxSteps}
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

func (d *Delegator) SetEvolvingMemory(em *memory.EvolvingMemory) {
	d.evolvingMemory = em
}

func (d *Delegator) ConfigureReMem(provider llm.Provider, model string, maxInnerSteps int) {
	d.reMemLLM = provider
	d.reMemModel = model
	d.reMemMaxSteps = maxInnerSteps
}

func (d *Delegator) SetBeliefMemory(store belief.Store) {
	d.beliefStore = store
}

func (d *Delegator) SetBeliefDistiller(distiller belief.Distiller) {
	d.beliefDistiller = distiller
}

func (d *Delegator) SetBeliefRetriever(retriever belief.Retriever, maxItems, maxTokens int) {
	d.beliefRetriever = retriever
	d.beliefMaxItems = maxItems
	d.beliefMaxTokens = maxTokens
}

func (d *Delegator) SetBeliefLifecycle(graph belief.Graph, promotionThreshold float64) {
	d.beliefGraph = graph
	d.beliefThreshold = promotionThreshold
}

func (d *Delegator) SetPolicyEnforcer(enforcer policy.Enforcer) {
	d.policyEnforcer = enforcer
}

func (d *Delegator) Run(ctx context.Context, req agent.DelegateRequest, tracer agent.AgentTracer) (string, error) {
	dispatchCtx := ctx
	if pid := req.ProjectID; pid != "" && d.wsMgr != nil {
		ws, err := d.wsMgr.Checkout(ctx, req.UserID, pid, "")
		if err != nil {
			return "", fmt.Errorf("workspace checkout failed: %w", err)
		}
		if ws.BaseDir != "" {
			dispatchCtx = sandbox.WithBaseDir(dispatchCtx, ws.BaseDir)
		}
	}

	var prov llm.Provider
	var toolsReg tools.Registry
	system := d.defaultSys
	model := ""
	imageGeneration := false

	toolsReg = d.reg
	inputRequestsEnabled := true

	if req.AgentName != "" && d.specReg != nil {
		if a, ok := d.specReg.Get(req.AgentName); ok && a != nil {
			prov = a.Provider()
			toolsReg = a.ToolsRegistry()
			inputRequestsEnabled = a.EnableTools
			imageGeneration = a.ImageGeneration
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
		inputRequestsEnabled = false
	} else if toolsReg == nil {
		toolsReg = tools.NewRegistry()
	}
	if inputRequestsEnabled {
		toolsReg = tools.NewOverlayRegistry(toolsReg, inputrequesttool.New())
	}
	if imageGeneration {
		toolsReg = tools.NewRegistry()
		system = ""
		maxSteps = 1
		req.History = nil
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
	if imageGeneration {
		runCtx = llm.WithImagePrompt(runCtx, llm.ImagePromptOptions{Size: defaultImagePromptSize})
	}
	runCtx = inputrequest.WithRunMetadata(runCtx, inputrequest.RunMetadata{
		Agent:        req.AgentName,
		Model:        model,
		CallID:       req.CallID,
		ParentCallID: req.ParentCallID,
		Depth:        req.Depth,
	})

	if tracer != nil {
		tracer.Trace(agent.AgentTrace{Type: "agent_start", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: req.Prompt})
	}

	eng := &agent.Engine{
		LLM:                       prov,
		Tools:                     toolsReg,
		MaxSteps:                  maxSteps,
		System:                    prompts.EnsureMemoryInstructions(system),
		Model:                     model,
		SessionID:                 req.SessionID,
		ProjectID:                 req.ProjectID,
		ObjectiveID:               req.ObjectiveID,
		UserID:                    req.UserID,
		AgentRole:                 req.AgentName,
		BeliefStore:               d.beliefStore,
		BeliefDistiller:           d.beliefDistiller,
		BeliefRetriever:           d.beliefRetriever,
		BeliefGraph:               d.beliefGraph,
		BeliefMaxBeliefsPerPrompt: d.beliefMaxItems,
		BeliefPromptTokenBudget:   d.beliefMaxTokens,
		BeliefPromotionThreshold:  d.beliefThreshold,
		PolicyEnforcer:            d.policyEnforcer,
		EvolvingMemory:            d.evolvingMemory,
		Delegator:                 d,
		AgentTracer:               tracer,
		AgentDepth:                req.Depth,
	}
	if imageGeneration {
		eng.System = ""
		eng.UserPromptContext = ""
		eng.EvolvingMemory = nil
		eng.Delegator = nil
		eng.BeliefStore = nil
		eng.BeliefDistiller = nil
		eng.BeliefRetriever = nil
		eng.BeliefGraph = nil
		eng.PolicyEnforcer = nil
		eng.SummaryEnabled = false
		eng.SkipInitialSummarization = true
	}
	if !imageGeneration && d.evolvingMemory != nil && d.reMemLLM != nil {
		eng.ReMemEnabled = true
		eng.ReMemController = memory.NewReMemController(memory.ReMemConfig{
			LLM:           d.reMemLLM,
			Model:         d.reMemModel,
			Memory:        d.evolvingMemory,
			MaxInnerSteps: d.reMemMaxSteps,
		})
	}
	eng.AttachTokenizer(prov, nil)

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
		eng.OnThoughtSummary = func(summary string) {
			if summary == "" {
				return
			}
			tracer.Trace(agent.AgentTrace{Type: "agent_thought_summary", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, ThoughtSummary: summary})
		}
	}

	observability.LoggerWithTrace(ctx).Info().Str("agent_delegate", req.AgentName).Msg("delegated_agent_start")
	// Use RunStream instead of Run to enable streaming callbacks (OnDelta, OnThoughtSummary, etc.)
	out, err := eng.RunStream(runCtx, req.Prompt, req.History)
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
