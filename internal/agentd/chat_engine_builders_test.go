package agentd

import (
	"context"
	"strings"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/config"
	"manifold/internal/llm"
	"manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	"manifold/internal/sandbox"
	"manifold/internal/specialists"
	"manifold/internal/testhelpers"
	"manifold/internal/tools"
)

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

	result := app.buildSpecialistChatEngine(ctx, "alpha", "override system", 7)
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

func TestBuildTeamChatEngineBuildsDelegatorAndDefaultPrompt(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	ctx := context.Background()

	_, err := app.specStore.Upsert(ctx, 9, persistence.Specialist{Name: "member-a", Provider: "openai", Model: "gpt-4.1-mini"})
	if err != nil {
		t.Fatalf("upsert specialist: %v", err)
	}
	_, err = app.teamStore.Upsert(ctx, 9, persistence.SpecialistTeam{
		Name: "ops",
		Orchestrator: persistence.Specialist{
			Name:        "ops-orchestrator",
			Provider:    "openai",
			EnableTools: true,
			AllowTools:  []string{"shell"},
		},
		Members: []string{"member-a"},
	})
	if err != nil {
		t.Fatalf("upsert team: %v", err)
	}

	result := app.buildTeamChatEngine(ctx, "ops", 9)
	if result.Err != nil {
		t.Fatalf("buildTeamChatEngine: %v", result.Err)
	}
	if result.Engine == nil || result.Engine.Delegator == nil {
		t.Fatal("expected team engine delegator to be configured")
	}
	if result.ModelLabel != "ops:gpt-4.1" {
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
}

func TestBuildOrchestratorChatEngineUsesOverride(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	result := app.buildOrchestratorChatEngine(context.Background(), 7, "sess-1", "override system", nil)
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

func TestBuildOrchestratorChatEngineDefaultsMaxSteps(t *testing.T) {
	t.Parallel()

	app := newChatEngineBuilderTestApp(t)
	app.cfg.MaxSteps = 0
	app.engine.MaxSteps = 0

	result := app.buildOrchestratorChatEngine(context.Background(), 7, "sess-1", "", nil)
	if result.Err != nil {
		t.Fatalf("buildOrchestratorChatEngine: %v", result.Err)
	}
	if result.Engine.MaxSteps != 8 {
		t.Fatalf("expected default max steps, got %d", result.Engine.MaxSteps)
	}
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
