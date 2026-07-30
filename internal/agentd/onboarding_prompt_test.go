package agentd

import (
	"context"
	"testing"

	"manifold/internal/config"
	"manifold/internal/persistence/databases"
	"manifold/internal/playground"
	"manifold/internal/playground/registry"
	"manifold/internal/specialists"
)

type onboardingPromptStore struct {
	prompts  map[string]registry.Prompt
	versions map[string][]registry.PromptVersion
}

func newOnboardingPromptStore() *onboardingPromptStore {
	return &onboardingPromptStore{prompts: map[string]registry.Prompt{}, versions: map[string][]registry.PromptVersion{}}
}

func (s *onboardingPromptStore) CreatePrompt(_ context.Context, prompt registry.Prompt) (registry.Prompt, error) {
	if _, exists := s.prompts[prompt.ID]; exists {
		return registry.Prompt{}, registry.ErrPromptExists
	}
	s.prompts[prompt.ID] = prompt
	return prompt, nil
}

func (s *onboardingPromptStore) GetPrompt(_ context.Context, id string) (registry.Prompt, bool, error) {
	prompt, ok := s.prompts[id]
	return prompt, ok, nil
}

func (s *onboardingPromptStore) ListPrompts(context.Context, registry.ListFilter) ([]registry.Prompt, error) {
	out := make([]registry.Prompt, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		out = append(out, prompt)
	}
	return out, nil
}

func (s *onboardingPromptStore) CreatePromptVersion(_ context.Context, version registry.PromptVersion) (registry.PromptVersion, error) {
	s.versions[version.PromptID] = append(s.versions[version.PromptID], version)
	return version, nil
}

func (s *onboardingPromptStore) ListPromptVersions(_ context.Context, promptID string) ([]registry.PromptVersion, error) {
	return append([]registry.PromptVersion(nil), s.versions[promptID]...), nil
}

func (s *onboardingPromptStore) GetPromptVersion(_ context.Context, id string) (registry.PromptVersion, bool, error) {
	for _, versions := range s.versions {
		for _, version := range versions {
			if version.ID == id {
				return version, true, nil
			}
		}
	}
	return registry.PromptVersion{}, false, nil
}

func (s *onboardingPromptStore) DeletePrompt(_ context.Context, id string) error {
	delete(s.prompts, id)
	delete(s.versions, id)
	return nil
}

func TestSeedOnboardingPromptIsIdempotentAndAssignsOrchestrator(t *testing.T) {
	ctx := context.Background()
	store := newOnboardingPromptStore()
	service := playground.NewService(playground.Config{}, playground.Dependencies{Registry: registry.New(store)})
	a := &app{
		cfg:               &config.Config{LLMClient: config.LLMClientConfig{Provider: "openai"}},
		playgroundService: service,
		specStore:         databases.NewSpecialistsStore(nil),
	}

	for range 2 {
		if err := a.seedOnboardingPrompt(ctx, 42); err != nil {
			t.Fatalf("seedOnboardingPrompt: %v", err)
		}
	}
	if len(store.prompts) != 1 {
		t.Fatalf("prompts = %d, want 1", len(store.prompts))
	}
	promptID, versionID := defaultPromptIDs(42)
	if len(store.versions[promptID]) != 1 || store.versions[promptID][0].Semver != "1.0" {
		t.Fatalf("versions = %#v, want one 1.0 version", store.versions[promptID])
	}
	sp, ok, err := a.specStore.GetByName(ctx, 42, specialists.OrchestratorName)
	if err != nil || !ok {
		t.Fatalf("load orchestrator: ok=%v err=%v", ok, err)
	}
	if sp.PromptID != promptID || sp.PromptVersionID != versionID || sp.System == "" {
		t.Fatalf("orchestrator prompt config = %+v", sp)
	}
}
