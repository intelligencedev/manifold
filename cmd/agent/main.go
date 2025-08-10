package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "time"

    "github.com/rs/zerolog/log"

    "gptagent/internal/agent"
    "gptagent/internal/agent/prompts"
    "gptagent/internal/config"
    openaillm "gptagent/internal/llm/openai"
    "gptagent/internal/observability"
    "gptagent/internal/tools"
    "gptagent/internal/tools/cli"
)

func main() {
    q := flag.String("q", "", "User request")
    maxSteps := flag.Int("max-steps", 8, "Max reasoning steps")
    flag.Parse()
    if *q == "" {
        fmt.Fprintln(os.Stderr, "usage: agent -q \"...\"")
        os.Exit(2)
    }

    cfg, err := config.Load()
    if err != nil {
        log.Fatal().Err(err).Msg("config")
    }

    observability.InitLogger()
    shutdown, _ := observability.InitOTel(context.Background(), cfg.Obs)
    defer func() { _ = shutdown(context.Background()) }()

    httpClient := observability.NewHTTPClient(nil)
    llm := openaillm.New(cfg.OpenAI, httpClient)

    registry := tools.NewRegistry()
    exec := cli.NewExecutor(cfg.Exec, cfg.Workdir)
    registry.Register(cli.NewTool(exec)) // provides run_cli

    eng := agent.Engine{
        LLM:      llm,
        Tools:    registry,
        MaxSteps: *maxSteps,
        System:   prompts.DefaultSystemPrompt(cfg.Workdir),
    }

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    final, err := eng.Run(ctx, *q, nil)
    if err != nil {
        log.Fatal().Err(err).Msg("agent")
    }
    fmt.Println(final)
}
