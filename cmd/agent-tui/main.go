package main

import (
    "context"
    "flag"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/rs/zerolog/log"

    "gptagent/internal/config"
    "gptagent/internal/llm/openai"
    "gptagent/internal/observability"
    "gptagent/internal/tools/cli"
    itui "gptagent/internal/tui"
)

func main() {
    maxSteps := flag.Int("max-steps", 8, "Max reasoning steps")
    warppFlag := flag.Bool("warpp", false, "Run WARPP workflow instead of LLM agent")
    flag.Parse()

    cfg, err := config.Load()
    if err != nil {
        log.Fatal().Err(err).Msg("config")
    }

    observability.InitLogger()
    shutdown, _ := observability.InitOTel(context.Background(), cfg.Obs)
    defer func() { _ = shutdown(context.Background()) }()

    httpClient := observability.NewHTTPClient(nil)
    provider := openai.New(cfg.OpenAI, httpClient)

    exec := cli.NewExecutor(cfg.Exec, cfg.Workdir)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    m := itui.NewModel(ctx, provider, cfg, exec, *maxSteps, *warppFlag)
    p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithMouseAllMotion())
    if _, err := p.Run(); err != nil {
        log.Error().Err(err).Msg("tui error")
        os.Exit(1)
    }
}
