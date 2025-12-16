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

	if got := app.engine.System; !strings.Contains(got, "alpha: desc") {
		t.Fatalf("expected system prompt to include specialist, got %q", got)
	}
	if got := app.engine.System; !strings.Contains(got, "Available specialists you can invoke:") {
		t.Fatalf("expected system prompt to include catalog header, got %q", got)
	}
}
