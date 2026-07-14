package chat

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"manifold/internal/agent"
	"manifold/internal/agent/harness"
	"manifold/internal/agent/memory"
	"manifold/internal/agent/prompts"
	"manifold/internal/sandbox"
	"manifold/internal/tools"
	"manifold/internal/workspaces"
)

// SanitizeImageGenerationBuild removes text-agent state before an image/video
// specialist is invoked.
func SanitizeImageGenerationBuild(build BuildResult) BuildResult {
	if !build.ImageGeneration && !build.VideoGeneration || build.Engine == nil {
		return build
	}
	build.Engine.System = ""
	build.Engine.UserPromptContext = ""
	build.Engine.Tools = tools.NewRegistry()
	build.Engine.MaxSteps = 1
	build.Engine.Delegator = nil
	build.Engine.TeamDelegator = nil
	build.Engine.BeliefStore = nil
	build.Engine.BeliefDistiller = nil
	build.Engine.BeliefRetriever = nil
	build.Engine.BeliefGraph = nil
	build.Engine.PolicyEnforcer = nil
	build.Engine.EvolvingMemory = nil
	build.Engine.Memory = nil
	build.Engine.DisableMemory = true
	build.Engine.DisableEvolvingMemory = true
	build.Engine.DisableBeliefMemory = true
	build.Engine.ReMemEnabled = false
	build.Engine.ReMemController = nil
	build.Engine.SummaryEnabled = false
	build.Engine.HarnessEnabled = false
	build.Engine.HarnessConfig = harness.RunConfig{}
	build.Engine.SkipInitialSummarization = true
	return build
}

// BuildOrchestrator constructs an orchestrator engine from the dependency
// slice.
func (d Deps) BuildOrchestrator(ctx context.Context, req BuildRequest) BuildResult {
	if d.Cfg == nil || d.CloneEngineForUser == nil {
		return BuildResult{StatusCode: http.StatusServiceUnavailable, Err: fmt.Errorf("agent unavailable")}
	}
	req.MemorySettings = NormalizeMemoryRunSettings(req.MemorySettings)
	d.ensureMCPWorkspaceSession(ctx, req.Owner, req.ProjectID, req.CheckedOutWorkspace)
	eng := d.CloneEngineForUser(ctx, req.Owner, req.SessionID, req.ProjectID, req.ObjectiveID, req.MemorySettings)
	if eng == nil {
		return BuildResult{StatusCode: http.StatusServiceUnavailable, Err: fmt.Errorf("agent unavailable")}
	}
	eng.MaxSteps = d.maxSteps()
	if override := strings.TrimSpace(req.SystemPromptOverride); override != "" && d.ComposeSystemPromptForUserWithOverride != nil {
		eng.System = d.ComposeSystemPromptForUserWithOverride(ctx, req.Owner, override)
	}
	eng.System = ensureMemoryInstructions(d.Cfg, eng.System, req.MemorySettings.EvolvingMemoryEnabled)
	enableTools, autoDiscover, requestInfo := d.orchestratorToolConfig(ctx, req.Owner)
	eng.System = ensureDiscoveryInstructions(d.Cfg, eng.System, enableTools, autoDiscover)
	eng.System = ensureRequestInfoInstructions(eng.System, enableTools, requestInfo)
	var skillsContext string
	eng.Tools, eng.System, skillsContext = applySkillsMode(d, eng.Tools, eng.System, d.projectDir(ctx, req.CheckedOutWorkspace), enableTools, autoDiscover)
	eng.Tools = withInputRequestTool(eng.Tools, enableTools && requestInfo)
	eng.UserPromptContext = combineUserPromptContext(eng.UserPromptContext, skillsContext)
	return BuildResult{Engine: eng, ModelLabel: eng.Model}
}

// BuildSpecialist constructs a specialist engine from the dependency slice.
func (d Deps) BuildSpecialist(ctx context.Context, req BuildRequest) BuildResult {
	if d.Cfg == nil || d.SpecialistsRegistryForUser == nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("specialist registry unavailable")}
	}
	req.MemorySettings = NormalizeMemoryRunSettings(req.MemorySettings)
	d.ensureMCPWorkspaceSession(ctx, req.Owner, req.ProjectID, req.CheckedOutWorkspace)
	reg, err := d.SpecialistsRegistryForUser(ctx, req.Owner)
	if err != nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("specialist registry unavailable: %w", err)}
	}
	sp, ok := reg.Get(req.Name)
	if !ok || sp == nil {
		return BuildResult{StatusCode: http.StatusNotFound, Err: fmt.Errorf("specialist not found: %s", req.Name)}
	}
	prov := sp.Provider()
	if prov == nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("specialist not configured: %s", req.Name)}
	}
	toolReg := sp.ToolsRegistry()
	if toolReg == nil || !sp.EnableTools {
		toolReg = tools.NewRegistry()
	}
	systemPrompt := sp.System
	if override := strings.TrimSpace(req.SystemPromptOverride); override != "" {
		systemPrompt = prompts.DefaultSystemPrompt(d.Cfg.Workdir, override, promptInstructionOverrides(d.Cfg))
	}
	systemPrompt = ensureMemoryInstructions(d.Cfg, systemPrompt, req.MemorySettings.EvolvingMemoryEnabled)
	systemPrompt = ensureDiscoveryInstructions(d.Cfg, systemPrompt, sp.EnableTools, sp.AutoDiscover)
	systemPrompt = ensureRequestInfoInstructions(systemPrompt, sp.EnableTools, sp.RequestInfoEnabled)
	var skillsContext string
	toolReg, systemPrompt, skillsContext = applySkillsMode(d, toolReg, systemPrompt, d.projectDir(ctx, nil), sp.EnableTools, sp.AutoDiscover)
	harnessCfg := harnessOverrideConfig(d, d.Cfg.Harness, sp.Harness)
	eng := &agent.Engine{
		LLM:                          prov,
		Tools:                        toolReg,
		MaxSteps:                     d.maxSteps(),
		System:                       systemPrompt,
		UserPromptContext:            combineUserPromptContext(sp.UserPromptContext, skillsContext),
		Model:                        sp.Model,
		ContextWindowTokens:          summaryContextSize(d, sp.SummaryContextWindowTokens, sp.Model),
		SummaryEnabled:               d.Cfg.SummaryEnabled,
		SummaryReserveBufferTokens:   d.Cfg.SummaryReserveBufferTokens,
		SummaryMinKeepLastMessages:   d.Cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: d.Cfg.SummaryMaxSummaryChunkTokens,
		SummaryCallTimeout:           summaryCallTimeout(d),
		HarnessEnabled:               harnessCfg.Enabled,
		HarnessConfig:                harnessRunConfig(d, harnessCfg),
	}
	if d.ConfigureBeliefRunState != nil {
		d.ConfigureBeliefRunState(eng, req.Owner, req.SessionID, req.ProjectID, req.ObjectiveID, req.Name)
	}
	var em *memory.EvolvingMemory
	if d.AttachSessionEvolvingMemory != nil {
		em = d.AttachSessionEvolvingMemory(eng, req.Owner, req.SessionID, req.MemorySettings.EvolvingMemoryEnabled)
	}
	ApplyMemorySettings(eng, req.MemorySettings)
	if eng.DisableEvolvingMemory {
		em = nil
	}
	if d.ConfigureUnifiedMemoryRuntime != nil {
		d.ConfigureUnifiedMemoryRuntime(eng, em, req.MemorySettings)
	}
	delegator := newDelegator(d, eng, reg, em)
	eng.Delegator = delegator
	eng.TeamDelegator = d.TeamDelegator
	return BuildResult{Engine: eng, ModelLabel: modelLabel(req.Name, sp.Model), ImageGeneration: sp.ImageGeneration, VideoGeneration: sp.VideoGeneration}
}

// BuildTeam constructs a team orchestrator engine from the dependency slice.
func (d Deps) BuildTeam(ctx context.Context, req BuildRequest) BuildResult {
	if d.Cfg == nil || d.TeamStore == nil || d.SpecialistsRegistryForUser == nil || d.ResolveTeamOrchestratorSpecialist == nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("teams unavailable")}
	}
	req.MemorySettings = NormalizeMemoryRunSettings(req.MemorySettings)
	d.ensureMCPWorkspaceSession(ctx, req.Owner, req.ProjectID, req.CheckedOutWorkspace)
	team, ok, err := d.TeamStore.GetByName(ctx, req.Owner, req.Name)
	if err != nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("failed to load team: %w", err)}
	}
	if !ok {
		return BuildResult{StatusCode: http.StatusNotFound, Err: fmt.Errorf("team not found: %s", req.Name)}
	}
	orchestratorSpec, statusCode, err := d.ResolveTeamOrchestratorSpecialist(ctx, req.Owner, team)
	if err != nil {
		return BuildResult{StatusCode: statusCode, Err: err}
	}
	orchestratorName := strings.TrimSpace(orchestratorSpec.Name)
	teamReg, err := d.buildTeamRegistry(ctx, req.Owner, team, orchestratorName)
	if err != nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: err}
	}
	reg, err := d.SpecialistsRegistryForUser(ctx, req.Owner)
	if err != nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("specialist registry unavailable: %w", err)}
	}
	sp, ok := reg.Get(orchestratorName)
	if !ok || sp == nil {
		return BuildResult{StatusCode: http.StatusBadRequest, Err: fmt.Errorf("team orchestrator specialist not available: %s", orchestratorName)}
	}
	userLLM := sp.Provider()
	if userLLM == nil {
		return BuildResult{StatusCode: http.StatusInternalServerError, Err: fmt.Errorf("team orchestrator not configured: %s", orchestratorName)}
	}
	currentModel := strings.TrimSpace(sp.Model)
	toolReg := sp.ToolsRegistry()
	if toolReg == nil || !sp.EnableTools {
		toolReg = tools.NewRegistry()
	}
	systemPrompt := ensureMemoryInstructions(d.Cfg, sp.System, req.MemorySettings.EvolvingMemoryEnabled)
	systemPrompt = ensureDiscoveryInstructions(d.Cfg, systemPrompt, sp.EnableTools, sp.AutoDiscover)
	systemPrompt = ensureRequestInfoInstructions(systemPrompt, sp.EnableTools, sp.RequestInfoEnabled)
	var skillsContext string
	toolReg, systemPrompt, skillsContext = applySkillsMode(d, toolReg, systemPrompt, d.projectDir(ctx, nil), sp.EnableTools, sp.AutoDiscover)
	harnessCfg := harnessOverrideConfig(d, d.Cfg.Harness, sp.Harness)
	eng := &agent.Engine{
		LLM:                          userLLM,
		Tools:                        toolReg,
		MaxSteps:                     d.maxSteps(),
		System:                       systemPrompt,
		UserPromptContext:            combineUserPromptContext(teamReg.UserPromptContext(), skillsContext),
		Model:                        currentModel,
		ContextWindowTokens:          summaryContextSize(d, sp.SummaryContextWindowTokens, currentModel),
		SummaryEnabled:               d.Cfg.SummaryEnabled,
		SummaryReserveBufferTokens:   d.Cfg.SummaryReserveBufferTokens,
		SummaryMinKeepLastMessages:   d.Cfg.SummaryMinKeepLastMessages,
		SummaryMaxSummaryChunkTokens: d.Cfg.SummaryMaxSummaryChunkTokens,
		SummaryCallTimeout:           summaryCallTimeout(d),
		HarnessEnabled:               harnessCfg.Enabled,
		HarnessConfig:                harnessRunConfig(d, harnessCfg),
	}
	if d.ConfigureBeliefRunState != nil {
		d.ConfigureBeliefRunState(eng, req.Owner, req.SessionID, req.ProjectID, req.ObjectiveID, orchestratorName)
	}
	var em *memory.EvolvingMemory
	if d.AttachSessionEvolvingMemory != nil {
		em = d.AttachSessionEvolvingMemory(eng, req.Owner, req.SessionID, req.MemorySettings.EvolvingMemoryEnabled)
	}
	ApplyMemorySettings(eng, req.MemorySettings)
	if eng.DisableEvolvingMemory {
		em = nil
	}
	if d.ConfigureUnifiedMemoryRuntime != nil {
		d.ConfigureUnifiedMemoryRuntime(eng, em, req.MemorySettings)
	}
	eng.AttachTokenizer(userLLM, nil)
	eng.Delegator = d.newTeamDelegator(eng, teamReg, em)
	eng.TeamDelegator = d.TeamDelegator
	return BuildResult{Engine: eng, ModelLabel: modelLabel(orchestratorName, currentModel), ImageGeneration: sp.ImageGeneration, VideoGeneration: sp.VideoGeneration}
}

func (d Deps) maxSteps() int {
	if d.Cfg == nil {
		return 0
	}
	return d.Cfg.MaxSteps
}

func (d Deps) projectDir(ctx context.Context, workspace *workspaces.Workspace) string {
	if workspace != nil && strings.TrimSpace(workspace.BaseDir) != "" {
		return workspace.BaseDir
	}
	if baseDir, ok := sandbox.BaseDirFromContext(ctx); ok {
		return baseDir
	}
	return ""
}
