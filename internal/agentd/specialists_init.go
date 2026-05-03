package agentd

import (
	"context"
	"manifold/internal/persistence/databases"
	"manifold/internal/specialists"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

func (a *app) initSpecialists(ctx context.Context) error {
	var pg *pgxpool.Pool
	if a.cfg.Databases.DefaultDSN != "" {
		if p, err := databases.OpenPool(ctx, a.cfg.Databases.DefaultDSN); err == nil {
			pg = p
		}
	}
	specStore := databases.NewSpecialistsStore(pg)
	_ = specStore.Init(ctx)
	a.specStore = specStore
	teamStore := databases.NewSpecialistTeamsStore(pg)
	_ = teamStore.Init(ctx)
	a.teamStore = teamStore

	if err := specialists.SeedStore(ctx, specStore, systemUserID, a.cfg.Specialists); err != nil {
		log.Warn().Err(err).Msg("seed specialists")
	}

	if list, err := specStore.List(ctx, systemUserID); err == nil {
		a.specRegistry.ReplaceFromConfigs(a.cfg.LLMClient, specialists.ConfigsFromStore(list), a.httpClient, a.baseToolRegistry)
		a.specRegistry.SetToolDiscovery(a.toolIndex, a.cfg.AutoDiscover, a.cfg.MaxDiscoveredTools)
	}
	a.refreshEngineSystemPrompt()

	if sp, ok, _ := specStore.GetByName(ctx, systemUserID, specialists.OrchestratorName); ok {
		if err := a.applyOrchestratorUpdate(ctx, sp); err != nil {
			log.Warn().Err(err).Msg("failed to apply orchestrator overlay")
		}
	} else {
		a.cfg.SystemPrompt = specialists.DefaultOrchestratorPrompt
		a.refreshEngineSystemPrompt()
	}

	return nil
}
