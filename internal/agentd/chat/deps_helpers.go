package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	agenttools "manifold/internal/tools/agents"
	"manifold/internal/workspaces"
)

func (d Deps) ensureMCPWorkspaceSession(ctx context.Context, owner int64, projectID string, workspace *workspaces.Workspace) {
	if d.MCPPool == nil || d.BaseToolRegistry == nil || !d.MCPPool.RequiresPerUserMCP() || owner == 0 {
		return
	}
	workspacePath := d.projectDir(ctx, workspace)
	if strings.TrimSpace(workspacePath) == "" {
		return
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		if id, ok := sandbox.ProjectIDFromContext(ctx); ok {
			projectID = strings.TrimSpace(id)
		}
	}
	if projectID == "" {
		return
	}
	if err := d.MCPPool.EnsureUserSession(ctx, d.BaseToolRegistry, owner, projectID, workspacePath); err != nil {
		log.Warn().Err(err).Int64("userID", owner).Str("projectID", projectID).Str("workspacePath", workspacePath).Msg("chat_mcp_session_ensure_failed")
	}
}

func (d Deps) orchestratorToolConfig(ctx context.Context, owner int64) (bool, bool, bool) {
	if d.Cfg == nil {
		return false, false, false
	}
	enableTools := d.Cfg.EnableTools
	autoDiscover := d.Cfg.AutoDiscover
	requestInfo := config.RequestInfoEnabled(d.Cfg.RequestInfoEnabled)
	if !d.Cfg.Auth.Enabled || owner == 0 || d.SpecStore == nil {
		return enableTools, autoDiscover, requestInfo
	}
	sp, ok, err := d.SpecStore.GetByName(ctx, owner, specialists.OrchestratorName)
	if err != nil || !ok {
		return enableTools, autoDiscover, requestInfo
	}
	enableTools = sp.EnableTools
	if sp.AutoDiscover != nil {
		autoDiscover = *sp.AutoDiscover
	}
	if sp.RequestInfoEnabled != nil {
		requestInfo = *sp.RequestInfoEnabled
	}
	return enableTools, autoDiscover, requestInfo
}

func (d Deps) newTeamDelegator(eng *agent.Engine, teamReg *specialists.Registry, em *memory.EvolvingMemory) *agenttools.Delegator {
	delegator := agenttools.NewDelegator(eng.Tools, teamReg, d.WorkspaceManager, d.maxSteps())
	if d.Cfg != nil {
		delegator.SetDefaultTimeout(d.Cfg.AgentRunTimeoutSeconds)
	}
	delegator.SetMemoryRuntime(eng.Memory)
	delegator.SetEvolvingMemory(em)
	delegator.SetBeliefMemory(eng.BeliefStore)
	delegator.SetBeliefDistiller(eng.BeliefDistiller)
	delegator.SetBeliefRetriever(eng.BeliefRetriever, eng.BeliefMaxBeliefsPerPrompt, eng.BeliefPromptTokenBudget)
	delegator.SetBeliefLifecycle(eng.BeliefGraph, eng.BeliefPromotionThreshold)
	delegator.SetPolicyEnforcer(eng.PolicyEnforcer)
	delegator.SetTeamDelegator(d.TeamDelegator)
	if eng.ReMemEnabled {
		delegator.ConfigureReMem(d.EvolvingConfig.LLM, d.EvolvingConfig.Model, d.ReMemMaxInnerSteps)
	}
	return delegator
}

func newDelegator(d Deps, eng *agent.Engine, reg *specialists.Registry, em *memory.EvolvingMemory) *agenttools.Delegator {
	return d.newTeamDelegator(eng, reg, em)
}

func (d Deps) buildTeamRegistry(ctx context.Context, owner int64, team persist.SpecialistTeam, excludeName string) (*specialists.Registry, error) {
	if d.Cfg == nil || d.SpecStore == nil {
		return nil, fmt.Errorf("specialists unavailable")
	}
	baseRegCfg := d.Cfg.LLMClient
	if orch, ok, _ := d.SpecStore.GetByName(ctx, owner, specialists.OrchestratorName); ok {
		baseRegCfg, _ = specialists.ApplyLLMClientOverride(baseRegCfg, orch)
	}
	memberSet := make(map[string]struct{}, len(team.Members))
	for _, member := range team.Members {
		key := strings.ToLower(strings.TrimSpace(member))
		if key == "" || strings.EqualFold(key, strings.TrimSpace(excludeName)) {
			continue
		}
		memberSet[key] = struct{}{}
	}
	list, err := d.SpecStore.List(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to load specialists: %w", err)
	}
	filtered := make([]persist.Specialist, 0, len(list))
	for _, specialist := range list {
		if _, ok := memberSet[strings.ToLower(strings.TrimSpace(specialist.Name))]; ok {
			filtered = append(filtered, specialist)
		}
	}
	reg := specialists.NewRegistry(baseRegCfg, specialists.ConfigsFromStore(filtered), d.HTTPClient, d.BaseToolRegistry)
	reg.SetMaxSteps(d.maxSteps())
	reg.SetPromptOverrides(promptInstructionOverrides(d.Cfg))
	reg.SetRequestInfoEnabled(config.RequestInfoEnabled(d.Cfg.RequestInfoEnabled))
	reg.SetToolDiscovery(d.ToolIndex, d.Cfg.AutoDiscover, d.Cfg.MaxDiscoveredTools)
	return reg, nil
}
