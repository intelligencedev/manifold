package agentd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/assets"
	"manifold/internal/config"
	"manifold/internal/observability"
	"manifold/internal/secrets"
	"manifold/internal/skills"
)

// installBundledSkills copies skills embedded in the binary into
// <manifoldHome>/skills, overwriting any that changed since the last release.
func installBundledSkills(home string) {
	sub, err := fs.Sub(assets.Skills, "skills")
	if err != nil {
		log.Warn().Err(err).Msg("bundled_skills_fs_unavailable")
		return
	}
	names, err := skills.InstallBundledSkills(sub, filepath.Join(home, "skills"))
	if err != nil {
		log.Warn().Err(err).Msg("bundled_skills_install_failed")
		return
	}
	log.Info().Strs("skills", names).Msg("bundled_skills_installed")
}

func Run() {
	if err := loadEnv(); err != nil {
		log.Debug().Err(err).Msg("no .env loaded")
	}

	// Create stable desktop dirs before config/log path materialization.
	if home, err := config.EnsureManifoldHome(); err != nil {
		log.Debug().Err(err).Msg("unable to create ~/.manifold")
	} else {
		installBundledSkills(home)
	}

	// Auto-provision the secrets key on first run so database-backed secret
	// storage can initialize without a manual MANIFOLD_SECRETS_KEY step. An
	// explicit env value (e.g. from .env) always takes precedence.
	if keyPath := config.DefaultSecretsKeyPath(); keyPath != "" {
		if created, err := secrets.EnsurePersistentKey(keyPath); err != nil {
			log.Warn().Err(err).Str("path", keyPath).Msg("unable to provision secrets key")
		} else if created {
			log.Info().Str("path", keyPath).Msg("generated persistent secrets key")
		}
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
		oauthBase := computeBaseOrigin(a.cfg.Auth.RedirectURL)
		if oauthBase == "" || oauthBase == a.cfg.Auth.RedirectURL {
			oauthBase = "http://localhost:32180"
		}
		go a.launchStartupMCPOAuthPrompts(oauthBase)
	}

	ln, bindAddr, err := listenHTTP(preferredListenPort)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to bind listen address")
	}
	baseURL := publicBaseURL(bindAddr)
	a.listenAddr = bindAddr
	a.publicURL = baseURL

	server := &http.Server{Handler: root}
	serverErr := make(chan error, 1)
	go func() {
		log.Info().Str("addr", bindAddr).Str("url", baseURL).Msg("agentd listening")
		serverErr <- server.Serve(ln)
	}()

	go a.openOnboardingBrowser(baseURL)

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

func (a *app) openOnboardingBrowser(baseURL string) {
	if a == nil || baseURL == "" {
		return
	}
	// Skip browser open when HEADLESS/CI is set.
	if strings.TrimSpace(os.Getenv("MANIFOLD_NO_BROWSER")) != "" || strings.TrimSpace(os.Getenv("CI")) != "" {
		return
	}
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	target := baseURL + "/"
	if a.cfg != nil && !config.HasPrimaryLLMCredentials(a.cfg) {
		target = baseURL + "/setup"
	}
	if err := openBrowser(target); err != nil {
		log.Debug().Err(err).Str("url", target).Msg("browser open skipped")
		return
	}
	log.Info().Str("url", target).Msg("opened browser")
}
