package agentd

import (
	"context"
	"strings"
	"testing"

	"manifold/internal/agent"
	"manifold/internal/config"
	"manifold/internal/persistence"
	"manifold/internal/specialists"
	"manifold/internal/tools"
)

type stubSpecialistsStore struct {
	list []persistence.Specialist
}

func (s *stubSpecialistsStore) Init(context.Context) error                  { return nil }
func (s *stubSpecialistsStore) Delete(context.Context, int64, string) error { return nil }
func (s *stubSpecialistsStore) Upsert(context.Context, int64, persistence.Specialist) (persistence.Specialist, error) {
	return persistence.Specialist{}, nil
}
func (s *stubSpecialistsStore) List(context.Context, int64) ([]persistence.Specialist, error) {
	return s.list, nil
}
func (s *stubSpecialistsStore) GetByName(ctx context.Context, userID int64, name string) (persistence.Specialist, bool, error) {
	for _, s := range s.list {
		if strings.EqualFold(s.Name, name) {
			return s, true, nil
		}
	}
	return persistence.Specialist{}, false, nil
}

func TestInvalidateSpecialistsCacheRefreshesSystemPrompt(t *testing.T) {
	cfg := config.Config{
		SystemPrompt: "base prompt",
		Workdir:      ".",
		Auth:         config.AuthConfig{Enabled: true},
		LLMClient:    config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{Model: "m"}},
	}
	baseTools := tools.NewRegistry()
	specReg := specialists.NewRegistry(cfg.LLMClient, nil, nil, baseTools)

	app := &app{
		cfg:              &cfg,
		specStore:        &stubSpecialistsStore{list: []persistence.Specialist{{Name: "alpha", Description: "desc", Model: "m"}}},
		specRegistry:     specReg,
		userSpecRegs:     map[int64]*specialists.Registry{systemUserID: specReg},
		baseToolRegistry: baseTools,
		httpClient:       nil,
		engine:           &agent.Engine{},
	}

	app.invalidateSpecialistsCache(context.Background(), systemUserID)

	if got := app.engine.UserPromptContext; !strings.Contains(got, "alpha: desc") {
		t.Fatalf("expected user prompt context to include specialist, got %q", got)
	}
	if got := app.engine.UserPromptContext; !strings.Contains(got, "Available specialists you can invoke:") {
		t.Fatalf("expected user prompt context to include catalog header, got %q", got)
	}
	if got := app.engine.System; strings.Contains(got, "alpha: desc") {
		t.Fatalf("did not expect specialist catalog in system prompt, got %q", got)
	}
}

func TestComposeSystemPromptForUserScopesCatalog(t *testing.T) {
	cfg := config.Config{
		SystemPrompt: "base prompt",
		Workdir:      ".",
		Auth:         config.AuthConfig{Enabled: true},
		LLMClient:    config.LLMClientConfig{Provider: "openai", OpenAI: config.OpenAIConfig{Model: "m"}},
	}
	baseTools := tools.NewRegistry()
	// System registry includes a system-only specialist.
	systemReg := specialists.NewRegistry(cfg.LLMClient, []config.SpecialistConfig{{Name: "sys", Description: "sys desc", Model: "m"}}, nil, baseTools)

	app := &app{
		cfg:              &cfg,
		specRegistry:     systemReg,
		userSpecRegs:     map[int64]*specialists.Registry{systemUserID: systemReg},
		baseToolRegistry: baseTools,
		httpClient:       nil,
		specStore:        &stubSpecialistsStore{list: []persistence.Specialist{{UserID: 123, Name: "user", Description: "user desc", Model: "m"}}},
	}

	// Non-system users should see only their catalog in user prompt context.
	prompt := app.composeSystemPromptForUser(context.Background(), 123)
	ctx := app.composeUserPromptContextForUser(context.Background(), 123)
	if strings.Contains(prompt, "sys: sys desc") || strings.Contains(prompt, "user: user desc") {
		t.Fatalf("expected system prompt to exclude catalogs, got %q", prompt)
	}
	if strings.Contains(ctx, "sys: sys desc") {
		t.Fatalf("expected non-system context to exclude system catalog, got %q", ctx)
	}
	if !strings.Contains(ctx, "user: user desc") {
		t.Fatalf("expected non-system context to include user catalog, got %q", ctx)
	}
}
