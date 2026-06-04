package agentd

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
	"manifold/internal/embeddedpg"
)

func newApp(ctx context.Context, cfg *config.Config) (*app, error) {
	cfg.Databases.EmbeddedDiagnosticLogPath = cfg.LogPath
	embeddedRuntime, err := embeddedpg.Start(&cfg.Databases)
	if err != nil {
		return nil, fmt.Errorf("start embedded postgres: %w", err)
	}
	defer func() {
		if err != nil && embeddedRuntime != nil {
			_ = embeddedRuntime.Stop()
		}
	}()

	startup, err := buildAppStartup(ctx, cfg)
	if err != nil {
		return nil, err
	}
	app := newAppShell(startup.shellDeps(cfg, embeddedRuntime))
	if err := app.initPersistentServices(ctx, cfg); err != nil {
		return nil, err
	}
	app.initEvolvingSessionJanitor(ctx, cfg)
	if err := app.initAgentRuntime(startup.agentRuntimeDeps(ctx, cfg)); err != nil {
		return nil, err
	}

	if err := app.initChatMemory(cfg, startup.mgr, startup.summaryLLM, startup.summaryModel); err != nil {
		return nil, err
	}
	if err := app.initPlaygroundServices(cfg, startup.mgr, startup.llm); err != nil {
		return nil, err
	}
	app.initProjectServices(cfg)

	if err := app.initAuth(ctx); err != nil {
		return nil, err
	}

	if err := app.initSpecialists(ctx); err != nil {
		return nil, err
	}

	app.initMetrics(ctx, cfg)

	_ = startup.routing.mcpManager // ensure lifetime; manager currently long-lived

	if startup.mgr.MCP != nil {
		ctxRefresh, cancelRefresh := context.WithTimeout(ctx, 30*time.Second)
		pendingAuthIDs, err := app.RefreshMCPServersOnStartup(ctxRefresh, systemUserID)
		if err != nil {
			log.Warn().Err(err).Msg("mcp_oauth_refresh_on_startup_failed")
		} else if !cfg.Auth.Enabled {
			app.startupMCPOAuthIDs = pendingAuthIDs
		}
		cancelRefresh()
	}
	app.syncWarppTools(context.Background())

	if err := app.matrixGateway.Start(ctx); err != nil {
		return nil, fmt.Errorf("start matrix gateway: %w", err)
	}
	app.pulseRuntime = newPulseRuntime(app, startup.mgr.Pulse)
	if err := app.pulseRuntime.Start(ctx); err != nil {
		return nil, fmt.Errorf("start matrix pulse runtime: %w", err)
	}
	app.durableWorker.Start(ctx)
	app.startDurableTaskJanitor(ctx, defaultDurableTaskJanitorInterval, defaultDurableTaskRetention)

	return app, nil
}
