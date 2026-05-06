package agentd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/config"
	"manifold/internal/observability"
)

func Run() {
	if err := loadEnv(); err != nil {
		log.Debug().Err(err).Msg("no .env loaded")
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		log.Fatal().Err(err).Msg("failed to load config")
	}

	observability.InitLogger(cfg.LogPath, cfg.LogLevel)

	shutdown, err := observability.InitOTel(context.Background(), cfg.Obs)
	if err != nil {
		log.Warn().Err(err).Msg("otel init failed, continuing without observability")
		shutdown = nil
	} else {
		// Bridge zerolog to OTLP log exporter
		observability.EnableOTelLogging(cfg.Obs.ServiceName)
	}
	if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	ctx := context.Background()
	a, err := newApp(ctx, &cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("initialization failed")
	}

	mux := newRouter(a)
	if err := a.registerFrontend(mux); err != nil {
		log.Error().Err(err).Msg("frontend registration failed")
	}

	root := a.wrapWithMiddleware(mux)
	if len(a.startupMCPOAuthIDs) > 0 {
		// Use the same base origin as the OAuth redirect_uri so that cookies set
		// during the bootstrap redirect are on the same host the auth server will
		// redirect back to (e.g. localhost vs 127.0.0.1 are different cookie origins).
		oauthBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
		if oauthBase == "" || oauthBase == a.cfg.Auth.RedirectURL {
			oauthBase = "http://localhost:32180"
		}
		go a.launchStartupMCPOAuthPrompts(oauthBase)
	}

	server := &http.Server{Addr: ":32180", Handler: root}
	serverErr := make(chan error, 1)
	go func() {
		log.Info().Msg("agentd listening on :32180")
		serverErr <- server.ListenAndServe()
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var runErr error
	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	case <-sigCtx.Done():
		log.Info().Msg("agentd shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Warn().Err(err).Msg("http server shutdown failed")
		_ = server.Close()
	}
	a.close()
	if runErr != nil {
		log.Fatal().Err(runErr).Msg("server failed")
	}
}
