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
	teamDelegator   agent.TeamDelegator
}

const defaultImagePromptSize = "1K"

type delegateRunConfig struct {
	dispatchCtx          context.Context
	provider             llm.Provider
	toolsReg             tools.Registry
	system               string
	model                string
	imageGeneration      bool
	inputRequestsEnabled bool
	maxSteps             int
	history              []llm.Message
}

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

func (d *Delegator) SetTeamDelegator(delegator agent.TeamDelegator) {
	d.teamDelegator = delegator
}

func (d *Delegator) Run(ctx context.Context, req agent.DelegateRequest, tracer agent.AgentTracer) (string, error) {
	dispatchCtx, err := d.dispatchContext(ctx, req)
	if err != nil {
		return "", err
	}
	cfg, err := d.runConfig(dispatchCtx, req)
	if err != nil {
		return "", fmt.Errorf("no llm provider available for delegated agent")
	}
	runCtx, cancel := d.runContext(cfg.dispatchCtx, req)
	if cancel != nil {
		defer cancel()
	}
	if cfg.imageGeneration {
		runCtx = llm.WithImagePrompt(runCtx, llm.ImagePromptOptions{Size: defaultImagePromptSize})
	}
	runCtx = inputrequest.WithRunMetadata(runCtx, inputrequest.RunMetadata{
		Agent:        req.AgentName,
		Model:        cfg.model,
		CallID:       req.CallID,
		ParentCallID: req.ParentCallID,
		Depth:        req.Depth,
	})
	d.traceStart(tracer, req, cfg)

	eng := d.newEngine(cfg, req, tracer)
	d.configureRuntimeFeatures(eng, cfg, req)
	d.attachTracer(eng, tracer, req, cfg.model)
	eng.AttachTokenizer(cfg.provider, nil)

	observability.LoggerWithTrace(ctx).Info().Str("agent_delegate", req.AgentName).Msg("delegated_agent_start")
	out, err := eng.RunStream(runCtx, req.Prompt, cfg.history)
	if err != nil {
		d.traceError(tracer, req, cfg.model, err)
		return "", err
	}
	d.traceFinal(tracer, req, cfg.model, out)
	return out, nil
}

func (d *Delegator) dispatchContext(ctx context.Context, req agent.DelegateRequest) (context.Context, error) {
	if req.ProjectID == "" || d.wsMgr == nil {
		return ctx, nil
	}
	ws, err := d.wsMgr.Checkout(ctx, req.UserID, req.ProjectID, "")
	if err != nil {
		return ctx, fmt.Errorf("workspace checkout failed: %w", err)
	}
	if ws.BaseDir == "" {
		return ctx, nil
	}
	return sandbox.WithBaseDir(ctx, ws.BaseDir), nil
}

func (d *Delegator) runConfig(dispatchCtx context.Context, req agent.DelegateRequest) (delegateRunConfig, error) {
	cfg := delegateRunConfig{
		dispatchCtx:          dispatchCtx,
		toolsReg:             d.reg,
		system:               d.defaultSys,
		inputRequestsEnabled: true,
		maxSteps:             d.maxSteps(req.MaxSteps),
		history:              req.History,
	}
	d.applySpecialistConfig(&cfg, req.AgentName)
	if cfg.provider == nil {
		cfg.provider = tools.ProviderFromContext(dispatchCtx)
	}
	if cfg.provider == nil {
		return cfg, fmt.Errorf("missing provider")
	}
	d.applyToolPolicy(&cfg, req)
	return cfg, nil
}

func (d *Delegator) applySpecialistConfig(cfg *delegateRunConfig, agentName string) {
	if agentName == "" || d.specReg == nil {
		return
	}
	a, ok := d.specReg.Get(agentName)
	if !ok || a == nil {
		return
	}
	cfg.provider = a.Provider()
	cfg.toolsReg = a.ToolsRegistry()
	cfg.inputRequestsEnabled = a.EnableTools
	cfg.imageGeneration = a.ImageGeneration
	cfg.system = a.System
	cfg.model = a.Model
	if a.EnableTools && cfg.toolsReg == nil {
		cfg.toolsReg = tools.NewRegistry()
	}
}

func (d *Delegator) maxSteps(requested int) int {
	if requested > 0 {
		return requested
	}
	if d.defaultMaxStep > 0 {
		return d.defaultMaxStep
	}
	return 8
}

func (d *Delegator) applyToolPolicy(cfg *delegateRunConfig, req agent.DelegateRequest) {
	if req.EnableTools != nil && !*req.EnableTools {
		cfg.toolsReg = tools.NewRegistry()
		cfg.inputRequestsEnabled = false
	} else if cfg.toolsReg == nil {
		cfg.toolsReg = tools.NewRegistry()
	}
	if cfg.inputRequestsEnabled {
		cfg.toolsReg = tools.NewOverlayRegistry(cfg.toolsReg, inputrequesttool.New())
	}
	if cfg.imageGeneration {
		cfg.toolsReg = tools.NewRegistry()
		cfg.system = ""
		cfg.maxSteps = 1
		cfg.history = nil
	}
}

func (d *Delegator) runContext(dispatchCtx context.Context, req agent.DelegateRequest) (context.Context, context.CancelFunc) {
	if req.TimeoutSeconds > 0 {
		return context.WithTimeout(dispatchCtx, time.Duration(req.TimeoutSeconds)*time.Second)
	}
	if _, has := dispatchCtx.Deadline(); !has && d.defaultTimeout > 0 {
		return context.WithTimeout(dispatchCtx, d.defaultTimeout)
	}
	return dispatchCtx, nil
}

func (d *Delegator) newEngine(cfg delegateRunConfig, req agent.DelegateRequest, tracer agent.AgentTracer) *agent.Engine {
	eng := &agent.Engine{
		LLM:                       cfg.provider,
		Tools:                     cfg.toolsReg,
		MaxSteps:                  cfg.maxSteps,
		System:                    prompts.EnsureMemoryInstructions(cfg.system),
		Model:                     cfg.model,
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
		DisableEvolvingMemory:     req.DisableEvolvingMemory,
		DisableBeliefMemory:       req.DisableBeliefMemory,
		Delegator:                 d,
		TeamDelegator:             d.teamDelegator,
		AgentTracer:               tracer,
		AgentDepth:                req.Depth,
	}
	return eng
}

func (d *Delegator) configureRuntimeFeatures(eng *agent.Engine, cfg delegateRunConfig, req agent.DelegateRequest) {
	if req.DisableBeliefMemory {
		eng.BeliefStore = nil
		eng.BeliefDistiller = nil
		eng.BeliefRetriever = nil
		eng.BeliefGraph = nil
		eng.PolicyEnforcer = nil
	}
	if req.DisableEvolvingMemory {
		eng.EvolvingMemory = nil
	}
	if cfg.imageGeneration {
		eng.System = ""
		eng.UserPromptContext = ""
		eng.EvolvingMemory = nil
		eng.DisableEvolvingMemory = true
		eng.Delegator = nil
		eng.TeamDelegator = nil
		eng.BeliefStore = nil
		eng.BeliefDistiller = nil
		eng.BeliefRetriever = nil
		eng.BeliefGraph = nil
		eng.PolicyEnforcer = nil
		eng.DisableBeliefMemory = true
		eng.SummaryEnabled = false
		eng.SkipInitialSummarization = true
	}
	if !cfg.imageGeneration && !req.DisableEvolvingMemory && d.evolvingMemory != nil && d.reMemLLM != nil {
		eng.ReMemEnabled = true
		eng.ReMemController = memory.NewReMemController(memory.ReMemConfig{
			LLM:           d.reMemLLM,
			Model:         d.reMemModel,
			Memory:        d.evolvingMemory,
			MaxInnerSteps: d.reMemMaxSteps,
		})
	}
}

func (d *Delegator) attachTracer(eng *agent.Engine, tracer agent.AgentTracer, req agent.DelegateRequest, model string) {
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
}

func (d *Delegator) traceStart(tracer agent.AgentTracer, req agent.DelegateRequest, cfg delegateRunConfig) {
	if tracer == nil {
		return
	}
	tracer.Trace(agent.AgentTrace{Type: "agent_start", Agent: req.AgentName, Model: cfg.model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: req.Prompt})
}

func (d *Delegator) traceError(tracer agent.AgentTracer, req agent.DelegateRequest, model string, err error) {
	if tracer == nil {
		return
	}
	tracer.Trace(agent.AgentTrace{Type: "agent_error", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Error: err.Error()})
}

func (d *Delegator) traceFinal(tracer agent.AgentTracer, req agent.DelegateRequest, model, out string) {
	if tracer == nil {
		return
	}
	tracer.Trace(agent.AgentTrace{Type: "agent_final", Agent: req.AgentName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: out})
}
