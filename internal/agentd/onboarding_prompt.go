package agentd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"manifold/internal/agentd/onboarding"
	"manifold/internal/defaultprompt"
	"manifold/internal/playground/registry"
	"manifold/internal/specialists"
)

func defaultPromptIDs(userID int64) (string, string) {
	return onboarding.PromptIDs(userID, defaultprompt.Name, defaultprompt.Version)
}

func (a *app) defaultPromptReference(ctx context.Context, userID int64) (string, string, error) {
	promptID, versionID := defaultPromptIDs(userID)
	if a.playgroundService == nil {
		return promptID, versionID, nil
	}
	prompts, err := a.playgroundService.ListPrompts(ctx, registry.ListFilter{Query: defaultprompt.Name, PerPage: 100})
	if err != nil {
		return "", "", err
	}
	for _, prompt := range prompts {
		if !strings.EqualFold(strings.TrimSpace(prompt.Name), defaultprompt.Name) {
			continue
		}
		versions, err := a.playgroundService.ListPromptVersions(ctx, prompt.ID)
		if err != nil {
			return "", "", err
		}
		for _, version := range versions {
			if strings.TrimSpace(version.Semver) == defaultprompt.Version {
				return prompt.ID, version.ID, nil
			}
		}
	}
	return promptID, versionID, nil
}

func (a *app) seedOnboardingPrompt(ctx context.Context, userID int64) error {
	promptID, versionID := defaultPromptIDs(userID)
	if a.playgroundService != nil {
		prompts, err := a.playgroundService.ListPrompts(ctx, registry.ListFilter{Query: defaultprompt.Name, PerPage: 100})
		if err != nil {
			return fmt.Errorf("list playground prompts: %w", err)
		}
		foundPrompt := false
		for _, prompt := range prompts {
			if strings.EqualFold(strings.TrimSpace(prompt.Name), defaultprompt.Name) {
				promptID = prompt.ID
				foundPrompt = true
				break
			}
		}
		if !foundPrompt {
			_, err = a.playgroundService.CreatePrompt(ctx, registry.Prompt{
				ID:          promptID,
				Name:        defaultprompt.Name,
				Description: "Default Manifold orchestrator and specialist prompt.",
				Tags:        []string{"manifold", "onboarding"},
			})
			if err != nil && !errors.Is(err, registry.ErrPromptExists) {
				return fmt.Errorf("create playground prompt: %w", err)
			}
		}

		versions, err := a.playgroundService.ListPromptVersions(ctx, promptID)
		if err != nil {
			return fmt.Errorf("list playground prompt versions: %w", err)
		}
		foundVersion := false
		for _, version := range versions {
			if strings.TrimSpace(version.Semver) == defaultprompt.Version {
				versionID = version.ID
				foundVersion = true
				break
			}
		}
		if !foundVersion {
			created, err := a.playgroundService.CreatePromptVersion(ctx, promptID, registry.PromptVersion{
				ID:        versionID,
				Semver:    defaultprompt.Version,
				Template:  defaultprompt.Content,
				CreatedBy: "onboarding",
			})
			if err != nil {
				return fmt.Errorf("create playground prompt version: %w", err)
			}
			versionID = created.ID
		}
	}

	if a.specStore == nil {
		return nil
	}
	sp, ok, err := a.specStore.GetByName(ctx, userID, specialists.OrchestratorName)
	if err != nil {
		return fmt.Errorf("load orchestrator prompt config: %w", err)
	}
	if !ok {
		sp = a.orchestratorSpecialist(ctx, userID)
	}
	sp.Name = specialists.OrchestratorName
	sp.PromptID = promptID
	sp.PromptVersionID = versionID
	sp.System = defaultprompt.Content
	if _, err := a.specStore.Upsert(ctx, userID, sp); err != nil {
		return fmt.Errorf("assign onboarding prompt to orchestrator: %w", err)
	}
	return nil
}
