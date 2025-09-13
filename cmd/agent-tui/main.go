package main

import (
	"context"
	"flag"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rs/zerolog/log"

	"intelligence.dev/internal/config"
	llmpkg "intelligence.dev/internal/llm"
	openaillm "intelligence.dev/internal/llm/openai"
	"intelligence.dev/internal/observability"
	"intelligence.dev/internal/tools/cli"
	itui "intelligence.dev/internal/tui"
)

func main() {
	// Load config first to get defaults
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config")
	}

	maxSteps := flag.Int("max-steps", cfg.MaxSteps, "Max reasoning steps")
	warppFlag := flag.Bool("warpp", false, "Run WARPP workflow instead of LLM agent")
	flag.Parse()

	observability.InitLogger(cfg.LogPath, cfg.LogLevel)
	shutdown, _ := observability.InitOTel(context.Background(), cfg.Obs)
	defer func() { _ = shutdown(context.Background()) }()

	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	// Configure global llm payload logging/truncation
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)
	provider := openaillm.New(cfg.OpenAI, httpClient)

	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := itui.NewModel(ctx, provider, cfg, exec, *maxSteps, *warppFlag)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		log.Error().Err(err).Msg("tui error")
		os.Exit(1)
	}
}
