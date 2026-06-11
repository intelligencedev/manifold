// Command memgym-sweep executes the Phase 3 coordinate-descent sweep over the
// Tier 1 memory-gym knob space. It is fully deterministic: every candidate
// configuration is evaluated with gym.RunSuite (no LLM, no network) and scored
// with gym.BuildScorecard. Guardrail failures reject a candidate outright;
// deterministic ties keep the incumbent (default) value. The frontier judge
// (claude_fable_5) is applied afterwards by the orchestrator to the finalists
// listed in the sweep summary — never inside this binary.
//
// Usage:
//
//	memgym-sweep -outdir memory-testing/results/phase3 -first-run 2 -finalists 5
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"manifold/internal/agent/memory/gym"
)

func main() {
	outdir := flag.String("outdir", "", "directory for per-run suite/scorecard artifacts and sweep-summary.json (required)")
	firstRun := flag.Int("first-run", 2, "run number for the base evaluation; candidates follow sequentially")
	finalists := flag.Int("finalists", 5, "maximum judge shortlist size (winner included)")
	suiteName := flag.String("suite", "phase3-sweep", "suite name recorded in results")
	flag.Parse()

	if *outdir == "" {
		fmt.Fprintln(os.Stderr, "memgym-sweep: -outdir is required")
		flag.Usage()
		os.Exit(2)
	}
	if err := os.MkdirAll(*outdir, 0o755); err != nil {
		fatal("create outdir: %v", err)
	}

	persist := func(run string, suite gym.SuiteResult, card gym.Scorecard) error {
		if err := gym.WriteResults(filepath.Join(*outdir, run+"-suite.json"), suite); err != nil {
			return err
		}
		return writeJSON(filepath.Join(*outdir, run+"-scorecard.json"), card)
	}

	res, err := gym.Sweep(context.Background(), gym.SweepRequest{
		SuiteName:    *suiteName,
		Base:         gym.DefaultKnobs(),
		Dimensions:   gym.Phase3Dimensions(),
		Evaluator:    gym.SuiteEvaluator(*suiteName, gym.Suite()),
		Persist:      persist,
		FirstRun:     *firstRun,
		MaxFinalists: *finalists,
	})
	// Persist whatever summary exists even on error so partial sweeps are
	// auditable, then fail loudly.
	summaryPath := filepath.Join(*outdir, "sweep-summary.json")
	if len(res.Outcomes) > 0 {
		if werr := writeJSON(summaryPath, res); werr != nil {
			fatal("write sweep summary: %v", werr)
		}
	}
	if err != nil {
		fatal("sweep: %v", err)
	}

	fmt.Printf("sweep complete: %d runs, winner=%s, finalists=%v\nsummary: %s\n",
		len(res.Outcomes), res.WinnerRun, res.Finalists, summaryPath)
}

func writeJSON(path string, v any) error {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return os.WriteFile(path, payload, 0o644)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "memgym-sweep: "+format+"\n", args...)
	os.Exit(1)
}
