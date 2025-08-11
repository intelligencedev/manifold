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
	llmpkg "gptagent/internal/llm"
	openaillm "gptagent/internal/llm/openai"
	"gptagent/internal/observability"
	"gptagent/internal/tools"
	"gptagent/internal/tools/cli"
	"gptagent/internal/tools/fs"
	llmtools "gptagent/internal/tools/llmtool"
	"gptagent/internal/tools/web"
	"gptagent/internal/warpp"
)

func main() {
	q := flag.String("q", "", "User request")
	maxSteps := flag.Int("max-steps", 8, "Max reasoning steps")
    warppFlag := flag.Bool("warpp", false, "Run WARPP workflow instead of LLM agent")
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
	registry.Register(cli.NewTool(exec))               // provides run_cli
	registry.Register(web.NewTool(cfg.Web.SearXNGURL)) // provides web_search
	registry.Register(web.NewFetchTool())              // provides web_fetch
	registry.Register(fs.NewWriteTool(cfg.Workdir))    // provides write_file
    // Provider factory for base_url override in llm_transform
    newProv := func(baseURL string) llmpkg.Provider {
        c2 := cfg.OpenAI
        c2.BaseURL = baseURL
        return openaillm.New(c2, httpClient)
    }
    registry.Register(llmtools.NewTransform(llm, cfg.OpenAI.Model, newProv)) // provides llm_transform

    // WARPP mode: run the WARPP workflow executor instead of the LLM loop
    if *warppFlag {
        wfreg, _ := warpp.LoadFromDir("configs/workflows")
        runner := &warpp.Runner{Workflows: wfreg, Tools: registry}
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
        defer cancel()
        intent := runner.DetectIntent(ctx, *q)
        wf, _ := wfreg.Get(intent)
        attrs := warpp.Attrs{"utter": *q}
        wfStar, _, attrs, err := runner.Personalize(ctx, wf, attrs)
        if err != nil { log.Fatal().Err(err).Msg("personalize") }
        allow := map[string]bool{}
        for _, s := range wfStar.Steps { if s.Tool != nil { allow[s.Tool.Name] = true } }
        final, err := runner.Execute(ctx, wfStar, allow, attrs)
        if err != nil { log.Fatal().Err(err).Msg("warpp") }
        fmt.Println(final)
        return
    }

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
