package agentd

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"manifold/internal/agent"
	"manifold/internal/agent/prompts"
	"manifold/internal/config"
	"manifold/internal/llm"
	llmproviders "manifold/internal/llm/providers"
	persist "manifold/internal/persistence"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	agenttools "manifold/internal/tools/agents"
	tooldiscovery "manifold/internal/tools/discovery"
	"manifold/internal/workspaces"
)

type chatEngineBuildResult struct {
	Engine     *agent.Engine
	ModelLabel string
	StatusCode int
	Err        error
}

func (a *app) chatMaxSteps() int {
	if a.cfg != nil && a.cfg.MaxSteps > 0 {
		return a.cfg.MaxSteps
	}
	return 8
}

func (a *app) buildOrchestratorChatEngine(ctx context.Context, owner int64, sessionID, systemPromptOverride string, checkedOutWorkspace *workspaces.Workspace) chatEngineBuildResult {
	eng := a.cloneEngineForUser(ctx, owner, sessionID)
	if eng == nil {
		return chatEngineBuildResult{StatusCode: http.StatusServiceUnavailable, Err: fmt.Errorf("agent unavailable")}
	}
	if eng.MaxSteps <= 0 {
		eng.MaxSteps = a.chatMaxSteps()
	}
	if override := strings.TrimSpace(systemPromptOverride); override != "" {
		eng.System = a.composeSystemPromptForUserWithOverride(ctx, owner, override)
	}
	enableTools, autoDiscover := a.chatOrchestratorToolConfig(ctx, owner)
	eng.System = a.ensureChatDiscoveryInstructions(eng.System, enableTools, autoDiscover)
	eng.Tools, eng.System = a.applyChatSkillsMode(eng.Tools, eng.System, a.chatProjectDir(ctx, checkedOutWorkspace), enableTools, autoDiscover)
	return chatEngineBuildResult{Engine: eng, ModelLabel: eng.Model}
}

func (a *app) buildSpecialistChatEngine(ctx context.Context, name, systemPromptOverride string, owner int64) chatEngineBuildResult {
	reg, err := a.specialistsRegistryForUser(ctx, owner)
	if err != nil {
		return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("specialist registry unavailable: %w", err)}
	}
	sp, ok := reg.Get(name)
	if !ok || sp == nil {
		return chatEngineBuildResult{StatusCode: http.StatusNotFound, Err: fmt.Errorf("specialist not found: %s", name)}
	}
	prov := sp.Provider()
	if prov == nil {
		return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("specialist not configured: %s", name)}
	}

	toolReg := sp.ToolsRegistry()
	if toolReg == nil || !sp.EnableTools {
		toolReg = tools.NewRegistry()
	}

	systemPrompt := prompts.EnsureMemoryInstructions(sp.System)
	if override := strings.TrimSpace(systemPromptOverride); override != "" {
		systemPrompt = prompts.EnsureMemoryInstructions(override)
	}
	systemPrompt = a.ensureChatDiscoveryInstructions(systemPrompt, sp.EnableTools, sp.AutoDiscover)
	toolReg, systemPrompt = a.applyChatSkillsMode(toolReg, systemPrompt, a.chatProjectDir(ctx, nil), sp.EnableTools, sp.AutoDiscover)

	eng := &agent.Engine{
		LLM:                          prov,
		Tools:                        toolReg,
		MaxSteps:                     a.chatMaxSteps(),
		System:                       systemPrompt,
		Model:                        sp.Model,
		ContextWindowTokens:          a.chatSummaryContextSize(sp.SummaryContextWindowTokens, sp.Model),
		SummaryEnabled:               a.cfg.SummaryEnabled,
		SummaryReserveBufferTokens:   a.cfg.SummaryReserveBufferTokens,
		SummaryMinKeepLastMessages:   a.cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: a.cfg.SummaryMaxSummaryChunkTokens,
	}
	eng.AttachTokenizer(prov, nil)

	return chatEngineBuildResult{
		Engine:     eng,
		ModelLabel: chatModelLabel(name, sp.Model),
	}
}

func (a *app) buildTeamChatEngine(ctx context.Context, name string, owner int64) chatEngineBuildResult {
	if a.teamStore == nil {
		return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("teams unavailable")}
	}
	team, ok, err := a.teamStore.GetByName(ctx, owner, name)
	if err != nil {
		return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("failed to load team: %w", err)}
	}
	if !ok {
		return chatEngineBuildResult{StatusCode: http.StatusNotFound, Err: fmt.Errorf("team not found: %s", name)}
	}

	sp := team.Orchestrator
	if strings.TrimSpace(sp.Name) == "" {
		sp.Name = specialists.OrchestratorName
	}

	teamReg, err := a.buildTeamRegistry(ctx, owner, team)
	if err != nil {
		return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: err}
	}

	llmCfg, provider := specialists.ApplyLLMClientOverride(a.cfg.LLMClient, sp)
	userCfg := *a.cfg
	userCfg.LLMClient = llmCfg
	if provider == "" || provider == "openai" || provider == "local" {
		userCfg.OpenAI = llmCfg.OpenAI
	}
	userLLM, err := llmproviders.Build(userCfg, a.httpClient)
	if err != nil {
		return chatEngineBuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("team orchestrator not configured: %w", err)}
	}

	currentModel := chatTeamModel(provider, llmCfg, sp)
	toolReg := a.chatToolRegistry(sp.EnableTools, sp.AllowTools, sp.AutoDiscover)
	basePrompt := strings.TrimSpace(sp.System)
	if basePrompt == "" {
		basePrompt = specialists.DefaultOrchestratorPrompt
	}
	systemPrompt := prompts.DefaultSystemPrompt(a.cfg.Workdir, basePrompt)
	resolvedAutoDiscover := a.resolveAutoDiscover(sp.AutoDiscover)
	systemPrompt = a.ensureChatDiscoveryInstructions(systemPrompt, sp.EnableTools, resolvedAutoDiscover)
	toolReg, systemPrompt = a.applyChatSkillsMode(toolReg, systemPrompt, a.chatProjectDir(ctx, nil), sp.EnableTools, resolvedAutoDiscover)
	systemPrompt = teamReg.AppendToSystemPrompt(systemPrompt)

	eng := &agent.Engine{
		LLM:                          userLLM,
		Tools:                        toolReg,
		MaxSteps:                     a.chatMaxSteps(),
		System:                       systemPrompt,
		Model:                        currentModel,
		ContextWindowTokens:          a.chatSummaryContextSize(sp.SummaryContextWindowTokens, currentModel),
		SummaryEnabled:               a.cfg.SummaryEnabled,
		SummaryReserveBufferTokens:   a.cfg.SummaryReserveBufferTokens,
		SummaryMinKeepLastMessages:   a.cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: a.cfg.SummaryMaxSummaryChunkTokens,
	}
	eng.AttachTokenizer(userLLM, nil)
	delegator := agenttools.NewDelegator(eng.Tools, teamReg, a.workspaceManager, a.chatMaxSteps())
	delegator.SetDefaultTimeout(a.cfg.AgentRunTimeoutSeconds)
	eng.Delegator = delegator

	return chatEngineBuildResult{
		Engine:     eng,
		ModelLabel: chatModelLabel(name, currentModel),
	}
}

func (a *app) buildTeamRegistry(ctx context.Context, owner int64, team persist.SpecialistTeam) (*specialists.Registry, error) {
	baseRegCfg := a.cfg.LLMClient
	if orch, ok, _ := a.specStore.GetByName(ctx, owner, specialists.OrchestratorName); ok {
		baseRegCfg, _ = specialists.ApplyLLMClientOverride(baseRegCfg, orch)
	}
	memberSet := make(map[string]struct{}, len(team.Members))
	for _, member := range team.Members {
		key := strings.ToLower(strings.TrimSpace(member))
		if key == "" {
			continue
		}
		memberSet[key] = struct{}{}
	}
	list, err := a.specStore.List(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to load specialists: %w", err)
	}
	filtered := make([]persist.Specialist, 0, len(list))
	for _, specialist := range list {
		if _, ok := memberSet[strings.ToLower(strings.TrimSpace(specialist.Name))]; ok {
			filtered = append(filtered, specialist)
		}
	}
	reg := specialists.NewRegistry(baseRegCfg, specialists.ConfigsFromStore(filtered), a.httpClient, a.baseToolRegistry)
	reg.SetToolDiscovery(a.toolIndex, a.cfg.AutoDiscover, a.cfg.MaxDiscoveredTools)
	return reg, nil
}

func (a *app) chatProjectDir(ctx context.Context, checkedOutWorkspace *workspaces.Workspace) string {
	if checkedOutWorkspace != nil && strings.TrimSpace(checkedOutWorkspace.BaseDir) != "" {
		return checkedOutWorkspace.BaseDir
	}
	if baseDir, ok := sandbox.BaseDirFromContext(ctx); ok {
		return baseDir
	}
	return ""
}

func (a *app) resolveAutoDiscover(autoDiscover *bool) bool {
	resolved := a.cfg.AutoDiscover
	if autoDiscover != nil {
		resolved = *autoDiscover
	}
	return resolved
}

func (a *app) ensureChatDiscoveryInstructions(systemPrompt string, enableTools bool, autoDiscover bool) string {
	if enableTools && autoDiscover {
		return prompts.EnsureToolDiscoveryInstructions(systemPrompt)
	}
	return systemPrompt
}

func (a *app) applyChatSkillsMode(toolReg tools.Registry, systemPrompt, projectDir string, enableTools, autoDiscover bool) (tools.Registry, string) {
	if strings.TrimSpace(projectDir) == "" {
		return toolReg, systemPrompt
	}
	if !enableTools || !autoDiscover {
		if skillsSection := prompts.RenderSkillsForProject(projectDir); skillsSection != "" {
			systemPrompt += "\n\n" + skillsSection
		}
		return toolReg, systemPrompt
	}
	cached, err := prompts.CachedSkillsForProject(projectDir)
	if err != nil || cached == nil || len(cached.Skills) == 0 {
		return toolReg, systemPrompt
	}
	systemPrompt = prompts.EnsureSkillDiscoveryInstructions(systemPrompt)
	if toolReg == nil {
		toolReg = tools.NewRegistry()
	}
	return tools.NewOverlayRegistry(toolReg, newSkillSearchTool(projectDir)), systemPrompt
}

func (a *app) chatOrchestratorToolConfig(ctx context.Context, owner int64) (bool, bool) {
	enableTools := a.cfg.EnableTools
	autoDiscover := a.cfg.AutoDiscover
	if !a.cfg.Auth.Enabled || owner == systemUserID || a.specStore == nil {
		return enableTools, autoDiscover
	}
	sp, ok, err := a.specStore.GetByName(ctx, owner, specialists.OrchestratorName)
	if err != nil || !ok {
		return enableTools, autoDiscover
	}
	if sp.EnableTools {
		enableTools = true
	} else {
		enableTools = false
	}
	if sp.AutoDiscover != nil {
		autoDiscover = *sp.AutoDiscover
	}
	return enableTools, autoDiscover
}

func (a *app) chatSummaryContextSize(configured int, model string) int {
	if configured > 0 {
		return configured
	}
	if a.cfg.SummaryContextWindowTokens > 0 {
		return a.cfg.SummaryContextWindowTokens
	}
	ctxSize, _ := llm.ContextSize(model)
	const defaultSummaryContextWindowCap = 32000
	if ctxSize <= 0 || ctxSize > defaultSummaryContextWindowCap {
		ctxSize = defaultSummaryContextWindowCap
	}
	return ctxSize
}

func (a *app) chatToolRegistry(enableTools bool, allowTools []string, autoDiscover *bool) tools.Registry {
	resolvedAutoDiscover := a.resolveAutoDiscover(autoDiscover)
	if resolvedAutoDiscover && enableTools && a.toolIndex != nil {
		return tooldiscovery.NewDiscoverableRegistry(a.baseToolRegistry, a.toolIndex, allowTools, a.cfg.MaxDiscoveredTools)
	}
	return tools.ApplyTopLevelPolicy(a.baseToolRegistry, enableTools, allowTools)
}

func chatModelLabel(name, model string) string {
	if strings.TrimSpace(model) == "" {
		return name
	}
	return fmt.Sprintf("%s:%s", name, model)
}

func chatTeamModel(provider string, llmCfg config.LLMClientConfig, sp persist.Specialist) string {
	currentModel := strings.TrimSpace(sp.Model)
	if currentModel != "" {
		return currentModel
	}
	switch provider {
	case "anthropic":
		return strings.TrimSpace(llmCfg.Anthropic.Model)
	case "google":
		return strings.TrimSpace(llmCfg.Google.Model)
	default:
		return strings.TrimSpace(llmCfg.OpenAI.Model)
	}
}
