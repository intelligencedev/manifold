package agentd

import (
	"context"
	"manifold/internal/config"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"

	"github.com/rs/zerolog/log"
)

func (a *app) initSpecialists(ctx context.Context) error {
	specStore := a.mgr.Specialists
	if specStore == nil {
		specStore = databases.NewSpecialistsStore(nil)
	}
	if err := specStore.Init(ctx); err != nil {
		return err
	}
	a.specStore = specStore
	teamStore := a.mgr.SpecialistTeams
	if teamStore == nil {
		teamStore = databases.NewSpecialistTeamsStore(nil)
	}
	if err := teamStore.Init(ctx); err != nil {
		return err
	}
	a.teamStore = teamStore

	if err := specialists.SeedStore(ctx, specStore, systemUserID, a.cfg.Specialists); err != nil {
		log.Warn().Err(err).Msg("seed specialists")
	}

	if list, err := specStore.List(ctx, systemUserID); err == nil {
		a.specRegistry.ReplaceFromConfigs(a.cfg.LLMClient, specialists.ConfigsFromStore(list), a.httpClient, a.baseToolRegistry)
		a.specRegistry.SetPromptOverrides(promptInstructionOverrides(a.cfg))
		a.specRegistry.SetRequestInfoEnabled(config.RequestInfoEnabled(a.cfg.RequestInfoEnabled))
		a.specRegistry.SetToolDiscovery(a.toolIndex, a.cfg.AutoDiscover, a.cfg.MaxDiscoveredTools)
	}
	a.refreshEngineSystemPrompt()

	if sp, ok, _ := specStore.GetByName(ctx, systemUserID, specialists.OrchestratorName); ok {
		if err := a.applyOrchestratorUpdate(ctx, sp); err != nil {
			log.Warn().Err(err).Msg("failed to apply orchestrator overlay")
		}
	} else {
		a.refreshEngineSystemPrompt()
	}

	return nil
}
