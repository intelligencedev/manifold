package agentd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/agent/harness"
	"manifold/internal/agent/memory"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
	tooldiscovery "manifold/internal/tools/discovery"
	"manifold/internal/workspaces"
)

func buildSpecialistTestRequest(name, systemPromptOverride, sessionID string, owner int64) chatEngineBuildRequest {
	return chatEngineBuildRequest{
		Name:                 name,
		SystemPromptOverride: systemPromptOverride,
		SessionID:            sessionID,
		Owner:                owner,
		MemorySettings:       defaultChatMemoryRunSettings(),
	}
}

func buildTeamTestRequest(name, sessionID string, owner int64) chatEngineBuildRequest {
	return chatEngineBuildRequest{
		Name:           name,
		SessionID:      sessionID,
		Owner:          owner,
		MemorySettings: defaultChatMemoryRunSettings(),
	}
}

func buildOrchestratorTestRequest(sessionID, systemPromptOverride string, owner int64, workspace *workspaces.Workspace) chatEngineBuildRequest {
	return chatEngineBuildRequest{
		SystemPromptOverride: systemPromptOverride,
		SessionID:            sessionID,
		Owner:                owner,
		CheckedOutWorkspace:  workspace,
		MemorySettings:       defaultChatMemoryRunSettings(),
	}
}

func enabledChatMemoryRunSettings() chatMemoryRunSettings {
	return chatMemoryRunSettings{
		MemoryEnabled:         true,
		EvolvingMemoryEnabled: true,
		BeliefMemoryEnabled:   true,
	}
}

func TestBuildSpecialistChatEngineUsesOverrideAndSkills(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	ctx := sandbox.WithBaseDir(context.Background(), t.TempDir())

	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:        "alpha",
		Provider:    "openai",
		Model:       "gpt-4.1-mini",
		System:      "specialist system",
		EnableTools: false,
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	app.invalidateSpecialistsCache(ctx, 7)

	req := buildSpecialistTestRequest("alpha", "override system", "sess-1", 7)
	req.MemorySettings = enabledChatMemoryRunSettings()
	result := app.buildSpecialistChatEngine(ctx, req)
	if result.Err != nil {
		t.Fatalf("buildSpecialistChatEngine: %v", result.Err)
	}
	if result.ModelLabel != "alpha:gpt-4.1-mini" {
		t.Fatalf("unexpected model label: %q", result.ModelLabel)
	}
	if got := result.Engine.System; !strings.Contains(got, "override system") {
		t.Fatalf("expected override system prompt, got %q", got)
	}
	if !strings.Contains(result.Engine.System, "[memory]") {
		t.Fatalf("expected memory instructions in system prompt, got %q", result.Engine.System)
	}
	if result.Engine.Tools == nil {
		t.Fatal("expected tool registry to be set")
	}
	if result.Engine.Model != "gpt-4.1-mini" {
		t.Fatalf("unexpected model: %q", result.Engine.Model)
	}
}

func TestChatToolRegistryExposesInputRequestWhenAllowListIsNarrow(t *testing.T) {
	t.Parallel()

	base := tools.NewRegistry()
	app := &app{
		cfg:              &config.Config{EnableTools: true},
		baseToolRegistry: base,
	}

	reg := app.chatToolRegistry(true, []string{"run_cli"}, nil, nil)
	names := tools.SchemaNames(reg)
	if !containsString(names, "request_info") {
		t.Fatalf("expected request_info to be exposed, got %v", names)
	}

	disabled := app.chatToolRegistry(false, []string{"request_info"}, nil, nil)
	if containsString(tools.SchemaNames(disabled), "request_info") {
		t.Fatal("did not expect request_info when tools are disabled")
	}

	requestInfoOff := false
	withoutRequestInfo := app.chatToolRegistry(true, []string{"run_cli"}, nil, &requestInfoOff)
	if containsString(tools.SchemaNames(withoutRequestInfo), "request_info") {
		t.Fatal("did not expect request_info when requestInfoEnabled is false")
	}
}

func TestBuildTeamChatEngineBuildsDelegatorAndDefaultPrompt(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	ctx := context.Background()

	_, err := app.specStore.Upsert(ctx, 9, persistence.Specialist{
		Name:        "lead",
		Provider:    "openai",
		Model:       "gpt-4.1",
		System:      specialists.DefaultOrchestratorPrompt,
		EnableTools: true,
		AllowTools:  []string{"shell"},
	})
	if err != nil {
		t.Fatalf("upsert lead specialist: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 9, persistence.Specialist{Name: "member-a", Provider: "openai", Model: "gpt-4.1-mini", Description: "worker member"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 9, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead", "member-a"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	result := app.buildTeamChatEngine(ctx, buildTeamTestRequest("ops", "sess-team", 9))
	if result.Err != nil {
		t.Fatalf("buildTeamChatEngine: %v", result.Err)
	}
	if result.Engine == nil || result.Engine.Delegator == nil {
		t.Fatal("expected team engine delegator to be configured")
	}
	if result.ModelLabel != "lead:gpt-4.1" {
		t.Fatalf("unexpected model label: %q", result.ModelLabel)
	}
	if !strings.Contains(result.Engine.System, specialists.DefaultOrchestratorPrompt) {
		t.Fatalf("expected default orchestrator prompt, got %q", result.Engine.System)
	}
	if result.Engine.Tools == nil {
		t.Fatal("expected team tool registry")
	}
	if result.Engine.ContextWindowTokens <= 0 {
		t.Fatalf("expected context window tokens, got %d", result.Engine.ContextWindowTokens)
	}
	if !strings.Contains(result.Engine.UserPromptContext, "member-a: worker member") {
		t.Fatalf("expected worker in team prompt context, got %q", result.Engine.UserPromptContext)
	}
	if strings.Contains(result.Engine.UserPromptContext, "lead:") {
		t.Fatalf("did not expect orchestrator in worker prompt context, got %q", result.Engine.UserPromptContext)
	}
}

func TestBuildTeamChatEngineUsesSelectedOrchestratorSpecialistConfig(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	ctx := context.Background()

	_, err := app.specStore.Upsert(ctx, 9, persistence.Specialist{
		Name:                       "lead",
		Provider:                   "openai",
		Model:                      "gpt-4.1-mini",
		System:                     "You are the OPS TEAM ORCHESTRATOR. Always answer with the ops runbook.",
		EnableTools:                true,
		AllowTools:                 []string{"shell"},
		SummaryContextWindowTokens: 12345,
	})
	if err != nil {
		t.Fatalf("upsert lead specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 9, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	result := app.buildTeamChatEngine(ctx, buildTeamTestRequest("ops", "sess-team", 9))
	if result.Err != nil {
		t.Fatalf("buildTeamChatEngine: %v", result.Err)
	}
	if result.Engine == nil {
		t.Fatal("expected team engine")
	}
	if !strings.Contains(result.Engine.System, "OPS TEAM ORCHESTRATOR") {
		t.Fatalf("expected team orchestrator system prompt, got %q", result.Engine.System)
	}
	if result.Engine.Model != "gpt-4.1-mini" {
		t.Fatalf("expected team orchestrator model, got %q", result.Engine.Model)
	}
	if result.ModelLabel != "lead:gpt-4.1-mini" {
		t.Fatalf("unexpected model label: %q", result.ModelLabel)
	}
	if result.Engine.ContextWindowTokens != 12345 {
		t.Fatalf("expected team context window 12345, got %d", result.Engine.ContextWindowTokens)
	}
}

func TestBuildOrchestratorChatEngineUsesOverride(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	result := app.buildOrchestratorChatEngine(context.Background(), buildOrchestratorTestRequest("sess-1", "override system", 7, nil))
	if result.Err != nil {
		t.Fatalf("buildOrchestratorChatEngine: %v", result.Err)
	}
	if result.Engine == nil {
		t.Fatal("expected orchestrator engine")
	}
	if result.ModelLabel != "orchestrator-model" {
		t.Fatalf("unexpected model label: %q", result.ModelLabel)
	}
	if got := result.Engine.System; !strings.Contains(got, "override system") {
		t.Fatalf("expected override in system prompt, got %q", got)
	}
}

func TestBuildOrchestratorChatEnginePreservesZeroMaxSteps(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.MaxSteps = 0
	app.engine.MaxSteps = 0

	result := app.buildOrchestratorChatEngine(context.Background(), buildOrchestratorTestRequest("sess-1", "", 7, nil))
	if result.Err != nil {
		t.Fatalf("buildOrchestratorChatEngine: %v", result.Err)
	}
	if result.Engine.MaxSteps != 0 {
		t.Fatalf("expected unbounded max steps, got %d", result.Engine.MaxSteps)
	}
}

func TestBuildChatEnginesCarryHarnessConfig(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.Harness = config.HarnessConfig{
		Enabled:           true,
		Mode:              "workflow",
		RescueEnabled:     true,
		MaxRetriesPerStep: 5,
		MaxToolErrors:     4,
		TerminalTools:     []string{"agent_response"},
		RequiredSteps:     []string{"search"},
		Compact: config.HarnessCompactConfig{
			Enabled:         true,
			KeepRecentSteps: 6,
			PhaseThresholds: []float64{0.5, 0.8},
		},
	}
	app.engine.HarnessEnabled = app.cfg.Harness.Enabled
	app.engine.HarnessConfig = harnessRunConfig(app.cfg.Harness)

	ctx := context.Background()
	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:        "alpha",
		Provider:    "openai",
		Model:       "gpt-4.1-mini",
		System:      "specialist system",
		EnableTools: true,
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 9, persistence.Specialist{
		Name:        "lead",
		Provider:    "openai",
		Model:       "gpt-4.1-mini",
		EnableTools: true,
	})
	if err != nil {
		t.Fatalf("upsert team orchestrator: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 9, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	orchestrator := app.buildOrchestratorChatEngine(ctx, buildOrchestratorTestRequest("sess-1", "", 7, nil))
	specialist := app.buildSpecialistChatEngine(ctx, buildSpecialistTestRequest("alpha", "", "sess-2", 7))
	team := app.buildTeamChatEngine(ctx, buildTeamTestRequest("ops", "sess-3", 9))

	for name, result := range map[string]chatEngineBuildResult{
		"orchestrator": orchestrator,
		"specialist":   specialist,
		"team":         team,
	} {
		if result.Err != nil {
			t.Fatalf("%s build error: %v", name, result.Err)
		}
		if result.Engine == nil || !result.Engine.HarnessEnabled {
			t.Fatalf("%s expected harness enabled, got %+v", name, result.Engine)
		}
		if result.Engine.HarnessConfig.Mode != harness.ModeWorkflow || result.Engine.HarnessConfig.MaxRetriesPerStep != 5 || result.Engine.HarnessConfig.MaxToolErrors != 4 {
			t.Fatalf("%s unexpected harness config: %+v", name, result.Engine.HarnessConfig)
		}
		if !result.Engine.HarnessConfig.RescueEnabled {
			t.Fatalf("%s expected rescue enabled", name)
		}
		if !reflect.DeepEqual(result.Engine.HarnessConfig.Workflow.RequiredSteps, []string{"search"}) {
			t.Fatalf("%s unexpected required steps: %+v", name, result.Engine.HarnessConfig.Workflow.RequiredSteps)
		}
		if !result.Engine.HarnessConfig.Compact.Enabled || result.Engine.HarnessConfig.Compact.KeepRecentSteps != 6 || !reflect.DeepEqual(result.Engine.HarnessConfig.Compact.PhaseThresholds, []float64{0.5, 0.8}) {
			t.Fatalf("%s unexpected compact config: %+v", name, result.Engine.HarnessConfig.Compact)
		}
	}
}

func TestBuildChatEnginesApplyPerTargetHarnessOverrides(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.Harness = config.HarnessConfig{
		Enabled:       false,
		Mode:          "guarded_chat",
		TerminalTools: []string{"agent_response"},
	}
	app.engine.HarnessEnabled = app.cfg.Harness.Enabled
	app.engine.HarnessConfig = harnessRunConfig(app.cfg.Harness)

	ctx := context.Background()
	override := &persistence.SpecialistHarness{
		Enabled:           true,
		Mode:              "workflow",
		MaxRetriesPerStep: 6,
		TerminalTools:     []string{"agent_response"},
		RequiredSteps:     []string{"search"},
	}
	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:        specialists.OrchestratorName,
		Provider:    "openai",
		Model:       "gpt-4.1-mini",
		EnableTools: true,
		Harness:     override,
	})
	if err != nil {
		t.Fatalf("upsert orchestrator: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:        "alpha",
		Provider:    "openai",
		Model:       "gpt-4.1-mini",
		EnableTools: true,
		Harness:     override,
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	app.invalidateSpecialistsCache(ctx, 7)
	_, err = app.specStore.Upsert(ctx, 9, persistence.Specialist{Name: "lead", Provider: "openai", Model: "gpt-4.1-mini", EnableTools: true, Harness: override})
	if err != nil {
		t.Fatalf("upsert team orchestrator: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 9, persistence.Specialist{Name: "member-a", Provider: "openai", Model: "gpt-4.1-mini"})
	if err != nil {
		t.Fatalf("upsert team member: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 9, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead", "member-a"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	orchestrator := app.buildOrchestratorChatEngine(ctx, buildOrchestratorTestRequest("sess-1", "", 7, nil))
	specialist := app.buildSpecialistChatEngine(ctx, buildSpecialistTestRequest("alpha", "", "sess-2", 7))
	team := app.buildTeamChatEngine(ctx, buildTeamTestRequest("ops", "sess-3", 9))

	for name, result := range map[string]chatEngineBuildResult{
		"orchestrator": orchestrator,
		"specialist":   specialist,
		"team":         team,
	} {
		if result.Err != nil {
			t.Fatalf("%s build error: %v", name, result.Err)
		}
		if result.Engine == nil || !result.Engine.HarnessEnabled {
			t.Fatalf("%s expected harness override enabled, got %+v", name, result.Engine)
		}
		if result.Engine.HarnessConfig.Mode != harness.ModeWorkflow || result.Engine.HarnessConfig.MaxRetriesPerStep != 6 {
			t.Fatalf("%s unexpected harness override: %+v", name, result.Engine.HarnessConfig)
		}
		if !reflect.DeepEqual(result.Engine.HarnessConfig.Workflow.RequiredSteps, []string{"search"}) {
			t.Fatalf("%s unexpected required steps: %+v", name, result.Engine.HarnessConfig.Workflow.RequiredSteps)
		}
	}
}

func TestBuildSpecialistChatEngineUsesSkillSearchWhenAutoDiscoverEnabled(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.AutoDiscover = true
	app.baseToolRegistry.Register(staticTool{name: "read_file", description: "Read files from disk"})
	app.toolIndex = tooldiscovery.NewToolIndex(app.baseToolRegistry.Schemas())
	ctx := sandbox.WithBaseDir(context.Background(), skillProjectDir(t, "pdf-context-builder", "Extract text and structure from PDF files."))

	autoDiscover := true
	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:         "alpha",
		Provider:     "openai",
		Model:        "gpt-4.1-mini",
		System:       "specialist system",
		EnableTools:  true,
		AutoDiscover: &autoDiscover,
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	app.invalidateSpecialistsCache(ctx, 7)

	result := app.buildSpecialistChatEngine(ctx, buildSpecialistTestRequest("alpha", "", "sess-1", 7))
	if result.Err != nil {
		t.Fatalf("buildSpecialistChatEngine: %v", result.Err)
	}
	if strings.Contains(result.Engine.System, "## Skills") {
		t.Fatalf("expected skill catalog to be deferred, got %q", result.Engine.System)
	}
	if !strings.Contains(result.Engine.System, "[skill_discovery]") {
		t.Fatalf("expected skill discovery instructions, got %q", result.Engine.System)
	}
	if !containsTool(result.Engine.Tools, "skill_search") {
		t.Fatalf("expected skill_search tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
	if !containsTool(result.Engine.Tools, "skill_read") {
		t.Fatalf("expected skill_read tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
}

func TestRefreshToolDiscoveryIndexMakesDynamicMCPToolSearchable(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.AutoDiscover = true
	app.cfg.EnableTools = true
	app.toolIndex = tooldiscovery.NewToolIndex(app.baseToolRegistry.Schemas())
	app.refreshOrchestratorToolRegistry()

	ctx := context.Background()
	autoDiscover := true
	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:         "alpha",
		Provider:     "openai",
		Model:        "gpt-4.1-mini",
		System:       "specialist system",
		EnableTools:  true,
		AutoDiscover: &autoDiscover,
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	if _, err := app.specialistsRegistryForUser(ctx, 7); err != nil {
		t.Fatalf("prime specialist registry: %v", err)
	}

	const dynamicTool = "oauth_mcp_lookup"
	app.baseToolRegistry.Register(staticTool{name: dynamicTool, description: "Lookup remote OAuth MCP resources"})
	if _, ok := app.toolIndex.Lookup(dynamicTool); ok {
		t.Fatalf("expected initial tool index to be stale before refresh")
	}

	app.refreshToolDiscoveryIndex()

	orchestrator := app.buildOrchestratorChatEngine(ctx, buildOrchestratorTestRequest("sess-1", "", 7, nil))
	if orchestrator.Err != nil {
		t.Fatalf("buildOrchestratorChatEngine: %v", orchestrator.Err)
	}
	assertToolSearchPromotes(t, orchestrator.Engine.Tools, dynamicTool)

	specialist := app.buildSpecialistChatEngine(ctx, buildSpecialistTestRequest("alpha", "", "sess-2", 7))
	if specialist.Err != nil {
		t.Fatalf("buildSpecialistChatEngine: %v", specialist.Err)
	}
	assertToolSearchPromotes(t, specialist.Engine.Tools, dynamicTool)
}

func TestBuildOrchestratorChatEngineFallsBackToInlineSkillsWhenToolsDisabled(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.AutoDiscover = true
	app.cfg.EnableTools = false
	projectDir := skillProjectDir(t, "deploy-runbook", "Deploy the application safely.")
	result := app.buildOrchestratorChatEngine(context.Background(), buildOrchestratorTestRequest("sess-1", "", 7, &workspaces.Workspace{BaseDir: projectDir}))
	if result.Err != nil {
		t.Fatalf("buildOrchestratorChatEngine: %v", result.Err)
	}
	if !strings.Contains(result.Engine.UserPromptContext, "## Skills") {
		t.Fatalf("expected inline skills in user prompt context when tools are disabled, got %q", result.Engine.UserPromptContext)
	}
	if strings.Contains(result.Engine.System, "## Skills") {
		t.Fatalf("did not expect inline skills in system prompt, got %q", result.Engine.System)
	}
	if containsTool(result.Engine.Tools, "skill_search") {
		t.Fatalf("did not expect skill_search tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
	if containsTool(result.Engine.Tools, "skill_read") {
		t.Fatalf("did not expect skill_read tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
}

func TestBuildOrchestratorChatEngineUsesSkillSearchWhenAutoDiscoverEnabled(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.AutoDiscover = true
	app.cfg.EnableTools = true
	app.baseToolRegistry.Register(staticTool{name: "read_file", description: "Read files from disk"})
	app.toolIndex = tooldiscovery.NewToolIndex(app.baseToolRegistry.Schemas())
	projectDir := skillProjectDir(t, "incident-response", "Handle production incidents with a runbook.")

	result := app.buildOrchestratorChatEngine(context.Background(), buildOrchestratorTestRequest("sess-1", "", 7, &workspaces.Workspace{BaseDir: projectDir}))
	if result.Err != nil {
		t.Fatalf("buildOrchestratorChatEngine: %v", result.Err)
	}
	if strings.Contains(result.Engine.System, "## Skills") {
		t.Fatalf("expected deferred skills in orchestrator prompt, got %q", result.Engine.System)
	}
	if !strings.Contains(result.Engine.System, "[skill_discovery]") {
		t.Fatalf("expected skill discovery instructions, got %q", result.Engine.System)
	}
	if !containsTool(result.Engine.Tools, "skill_search") {
		t.Fatalf("expected skill_search tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
	if !containsTool(result.Engine.Tools, "skill_read") {
		t.Fatalf("expected skill_read tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
}

func TestBuildOrchestratorChatEngineUsesInlineSkillsAndSkillReadWhenAutoDiscoverDisabled(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.AutoDiscover = false
	app.cfg.EnableTools = true
	projectDir := skillProjectDir(t, "release-checklist", "Coordinate a production release checklist.")

	result := app.buildOrchestratorChatEngine(context.Background(), buildOrchestratorTestRequest("sess-1", "", 7, &workspaces.Workspace{BaseDir: projectDir}))
	if result.Err != nil {
		t.Fatalf("buildOrchestratorChatEngine: %v", result.Err)
	}
	if !strings.Contains(result.Engine.UserPromptContext, "## Skills") {
		t.Fatalf("expected inline skills, got %q", result.Engine.UserPromptContext)
	}
	if containsTool(result.Engine.Tools, "skill_search") {
		t.Fatalf("did not expect skill_search tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
	if !containsTool(result.Engine.Tools, "skill_read") {
		t.Fatalf("expected skill_read tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
}

func TestBuildTeamChatEngineUsesSkillSearchWhenAutoDiscoverEnabled(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.AutoDiscover = true
	app.baseToolRegistry.Register(staticTool{name: "read_file", description: "Read files from disk"})
	app.toolIndex = tooldiscovery.NewToolIndex(app.baseToolRegistry.Schemas())
	ctx := sandbox.WithBaseDir(context.Background(), skillProjectDir(t, "release-checklist", "Coordinate a production release checklist."))

	autoDiscover := true
	_, err := app.specStore.Upsert(ctx, 9, persistence.Specialist{
		Name:         "lead",
		Provider:     "openai",
		Model:        "gpt-4.1-mini",
		EnableTools:  true,
		AutoDiscover: &autoDiscover,
		AllowTools:   []string{"read_file"},
	})
	if err != nil {
		t.Fatalf("upsert team orchestrator: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 9, persistence.Specialist{Name: "member-a", Provider: "openai", Model: "gpt-4.1-mini"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 9, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead", "member-a"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	result := app.buildTeamChatEngine(ctx, buildTeamTestRequest("ops", "sess-team", 9))
	if result.Err != nil {
		t.Fatalf("buildTeamChatEngine: %v", result.Err)
	}
	if strings.Contains(result.Engine.System, "## Skills") {
		t.Fatalf("expected deferred skills in team prompt, got %q", result.Engine.System)
	}
	if !strings.Contains(result.Engine.System, "[skill_discovery]") {
		t.Fatalf("expected skill discovery instructions, got %q", result.Engine.System)
	}
	if !containsTool(result.Engine.Tools, "skill_search") {
		t.Fatalf("expected skill_search tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
	if !containsTool(result.Engine.Tools, "skill_read") {
		t.Fatalf("expected skill_read tool, got %v", tools.SchemaNames(result.Engine.Tools))
	}
}

func TestBuildSpecialistChatEngineAttachesSessionEvolvingMemory(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.evolvingCfg = memory.EvolvingMemoryConfig{LLM: app.llm}
	ctx := sandbox.WithBaseDir(context.Background(), t.TempDir())

	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:        "alpha",
		Provider:    "openai",
		Model:       "gpt-4.1-mini",
		System:      "specialist system",
		EnableTools: true,
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	app.invalidateSpecialistsCache(ctx, 7)

	req := buildSpecialistTestRequest("alpha", "", "sess-42", 7)
	req.MemorySettings = enabledChatMemoryRunSettings()
	result := app.buildSpecialistChatEngine(ctx, req)
	if result.Err != nil {
		t.Fatalf("buildSpecialistChatEngine: %v", result.Err)
	}
	if result.Engine.EvolvingMemory == nil {
		t.Fatal("expected specialist engine evolving memory")
	}
	if result.Engine.SessionID != "sess-42" {
		t.Fatalf("expected session id sess-42, got %q", result.Engine.SessionID)
	}
	if result.Engine.Delegator == nil {
		t.Fatal("expected specialist delegator")
	}
}

func TestBuildSpecialistChatEngineCanDisableSessionEvolvingMemory(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.evolvingCfg = memory.EvolvingMemoryConfig{LLM: app.llm}
	app.engine.ReMemEnabled = true
	ctx := sandbox.WithBaseDir(context.Background(), t.TempDir())

	_, err := app.specStore.Upsert(ctx, 7, persistence.Specialist{
		Name:        "alpha",
		Provider:    "openai",
		Model:       "gpt-4.1-mini",
		System:      "specialist system",
		EnableTools: true,
	})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	app.invalidateSpecialistsCache(ctx, 7)

	result := app.buildSpecialistChatEngine(ctx, chatEngineBuildRequest{
		Name:      "alpha",
		SessionID: "sess-42",
		Owner:     7,
		MemorySettings: chatMemoryRunSettings{
			EvolvingMemoryEnabled: false,
			BeliefMemoryEnabled:   true,
		},
	})
	if result.Err != nil {
		t.Fatalf("buildSpecialistChatEngine: %v", result.Err)
	}
	if result.Engine.EvolvingMemory != nil || result.Engine.ReMemEnabled || result.Engine.ReMemController != nil {
		t.Fatalf("expected evolving memory and ReMem disabled, got memory=%v remem=%v controller=%v", result.Engine.EvolvingMemory, result.Engine.ReMemEnabled, result.Engine.ReMemController)
	}
	if !result.Engine.DisableEvolvingMemory {
		t.Fatal("expected DisableEvolvingMemory flag")
	}
	if strings.Contains(result.Engine.System, "[memory]") {
		t.Fatalf("did not expect memory instructions when evolving memory is disabled: %q", result.Engine.System)
	}
}

func TestApplyChatMemorySettingsCanDisableBeliefMemory(t *testing.T) {
	t.Parallel()

	eng := &agent.Engine{
		BeliefMaxBeliefsPerPrompt: 4,
		BeliefPromptTokenBudget:   700,
		BeliefPromotionThreshold:  0.8,
	}
	applyChatMemorySettingsToEngine(eng, chatMemoryRunSettings{
		EvolvingMemoryEnabled: true,
		BeliefMemoryEnabled:   false,
	})

	if !eng.DisableBeliefMemory {
		t.Fatal("expected DisableBeliefMemory flag")
	}
	if eng.BeliefMaxBeliefsPerPrompt != 0 || eng.BeliefPromptTokenBudget != 0 || eng.BeliefPromotionThreshold != 0 {
		t.Fatalf("expected belief runtime settings cleared, got %+v", eng)
	}
}

func TestBuildTeamChatEngineAttachesSessionEvolvingMemory(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.evolvingCfg = memory.EvolvingMemoryConfig{LLM: app.llm}
	ctx := context.Background()

	_, err := app.specStore.Upsert(ctx, 9, persistence.Specialist{Name: "lead", Provider: "openai", Model: "gpt-4.1-mini", EnableTools: true})
	if err != nil {
		t.Fatalf("upsert team orchestrator: %v", err)
	}
	_, err = app.specStore.Upsert(ctx, 9, persistence.Specialist{Name: "member-a", Provider: "openai", Model: "gpt-4.1-mini"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 9, persistence.SpecialistTeam{
		Name:             "ops",
		OrchestratorName: "lead",
		Members:          []string{"lead", "member-a"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	req := buildTeamTestRequest("ops", "sess-team", 9)
	req.MemorySettings = enabledChatMemoryRunSettings()
	result := app.buildTeamChatEngine(ctx, req)
	if result.Err != nil {
		t.Fatalf("buildTeamChatEngine: %v", result.Err)
	}
	if result.Engine.EvolvingMemory == nil {
		t.Fatal("expected team engine evolving memory")
	}
	if result.Engine.SessionID != "sess-team" {
		t.Fatalf("expected session id sess-team, got %q", result.Engine.SessionID)
	}
	if result.Engine.Delegator == nil {
		t.Fatal("expected team delegator")
	}
}

type staticTool struct {
	name        string
	description string
}

func (t staticTool) Name() string { return t.name }

func (t staticTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.name,
		"description": t.description,
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t staticTool) Call(context.Context, json.RawMessage) (any, error) {
	return map[string]any{"ok": true}, nil
}

func containsTool(reg tools.Registry, name string) bool {
	return slices.Contains(tools.SchemaNames(reg), name)
}

func assertToolSearchPromotes(t *testing.T, reg tools.Registry, name string) {
	t.Helper()
	if reg == nil {
		t.Fatal("expected tool registry")
	}
	if containsTool(reg, name) {
		t.Fatalf("expected %s to be hidden before search, got %v", name, tools.SchemaNames(reg))
	}
	raw, err := json.Marshal(map[string]any{"names": []string{name}})
	if err != nil {
		t.Fatalf("marshal tool_search args: %v", err)
	}
	payload, err := reg.Dispatch(context.Background(), "tool_search", raw)
	if err != nil {
		t.Fatalf("tool_search dispatch: %v", err)
	}
	if !containsTool(reg, name) {
		t.Fatalf("expected %s to be promoted after tool_search; payload=%s tools=%v", name, payload, tools.SchemaNames(reg))
	}
}

func skillProjectDir(t *testing.T, name, description string) string {
	t.Helper()
	projectDir := t.TempDir()
	skillPath := filepath.Join(projectDir, "skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	content := strings.Join([]string{
		"---",
		"name: " + name,
		"description: " + description,
		"metadata:",
		"  short-description: " + description,
		"---",
		"# " + name,
	}, "\n")
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return projectDir
}

func newChatEngineBuilderTestApp(t *testing.T) *app {
	t.Helper()

	baseTools := tools.NewRegistry()
	baseProvider := &testhelpers.FakeProvider{Resp: llm.Message{Role: "assistant", Content: "ok"}}
	return &app{
		cfg: &config.Config{
			Workdir:        ".",
			EnableTools:    true,
			SystemPrompt:   "base system",
			MaxSteps:       8,
			LLMClient:      config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{Model: "gpt-4.1", BaseURL: "https://api.example.com", APIKey: "secret"}},
			SummaryEnabled: true,
			Auth:           config.AuthConfig{Enabled: true},
		},
		httpClient:       nil,
		llm:              baseProvider,
		baseToolRegistry: baseTools,
		specStore:        databases.NewSpecialistsStore(nil),
		teamStore:        databases.NewSpecialistTeamsStore(nil),
		specRegistry:     specialists.NewRegistry(config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{Model: "gpt-4.1"}}, nil, nil, baseTools),
		userSpecRegs:     map[int64]*specialists.Registry{},
		engine: &agent.Engine{
			LLM:    baseProvider,
			Tools:  baseTools,
			Model:  "orchestrator-model",
			System: "base orchestrator system",
		},
	}
}
