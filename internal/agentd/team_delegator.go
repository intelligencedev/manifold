package agentd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"manifold/internal/agent"
	"manifold/internal/observability"
)

type teamTraceAnnotator struct {
	team string
	next agent.AgentTracer
}

func (t teamTraceAnnotator) Trace(ev agent.AgentTrace) {
	if strings.TrimSpace(ev.Team) == "" {
		ev.Team = t.team
	}
	if t.next != nil {
		t.next.Trace(ev)
	}
}

func (a *app) RunTeam(ctx context.Context, req agent.TeamDelegateRequest, tracer agent.AgentTracer) (string, error) {
	teamName := strings.TrimSpace(req.TeamName)
	if teamName == "" {
		return "", fmt.Errorf("team is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}

	memoryEnabled := !req.DisableEvolvingMemory && !req.DisableBeliefMemory
	settings := chatMemoryRunSettings{
		MemoryEnabled:         memoryEnabled,
		EvolvingMemoryEnabled: memoryEnabled,
		BeliefMemoryEnabled:   memoryEnabled,
	}
	build := a.buildTeamChatEngine(ctx, chatEngineBuildRequest{
		Name:           teamName,
		SessionID:      req.SessionID,
		ProjectID:      req.ProjectID,
		ObjectiveID:    req.ObjectiveID,
		Owner:          req.UserID,
		MemorySettings: settings,
	})
	if build.Err != nil {
		return "", build.Err
	}
	if build.Engine == nil {
		return "", fmt.Errorf("team engine unavailable")
	}

	eng := build.Engine
	eng.AgentDepth = req.Depth
	eng.TeamDelegator = a
	model := strings.TrimSpace(eng.Model)
	orchestratorAgent := strings.TrimSpace(eng.AgentRole)
	if orchestratorAgent == "" {
		orchestratorAgent = "orchestrator"
	}
	annotatedTracer := teamTraceAnnotator{team: teamName, next: tracer}

	runCtx := ctx
	if req.TimeoutMS > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond)
		defer cancel()
	} else if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	if tracer != nil {
		annotatedTracer.Trace(agent.AgentTrace{Type: "agent_start", Agent: orchestratorAgent, Team: teamName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: req.Prompt})
		eng.AgentTracer = annotatedTracer
		eng.OnDelta = func(delta string) {
			if delta == "" {
				return
			}
			annotatedTracer.Trace(agent.AgentTrace{Type: "agent_delta", Agent: orchestratorAgent, Team: teamName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: delta, Role: "assistant"})
		}
		eng.OnToolStart = func(name string, args []byte, toolID string) {
			annotatedTracer.Trace(agent.AgentTrace{Type: "agent_tool_start", Agent: orchestratorAgent, Team: teamName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Title: name, Args: string(args), ToolID: toolID})
		}
		eng.OnTool = func(name string, args []byte, result []byte, toolID string) {
			annotatedTracer.Trace(agent.AgentTrace{Type: "agent_tool_result", Agent: orchestratorAgent, Team: teamName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Title: name, Args: string(args), Data: string(result), ToolID: toolID})
		}
		eng.OnThoughtSummary = func(summary string) {
			if strings.TrimSpace(summary) == "" {
				return
			}
			annotatedTracer.Trace(agent.AgentTrace{Type: "agent_thought_summary", Agent: orchestratorAgent, Team: teamName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, ThoughtSummary: summary})
		}
	}

	observability.LoggerWithTrace(ctx).Info().Str("team_delegate", teamName).Msg("delegated_team_start")
	out, err := eng.RunStream(runCtx, req.Prompt, req.History)
	if err != nil {
		if tracer != nil {
			annotatedTracer.Trace(agent.AgentTrace{Type: "agent_error", Agent: orchestratorAgent, Team: teamName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Error: err.Error()})
		}
		return "", err
	}
	if tracer != nil {
		annotatedTracer.Trace(agent.AgentTrace{Type: "agent_final", Agent: orchestratorAgent, Team: teamName, Model: model, CallID: req.CallID, ParentCallID: req.ParentCallID, Depth: req.Depth, Content: out})
	}
	return out, nil
}
