// Command memgym-score converts a Tier 1 memory-gym SuiteResult JSON file
// into a Phase 2 scorecard JSON artifact. Scoring is fully deterministic;
// the qualitative judge verdict (claude_fable_5) is attached later as data
// by the orchestrator via the optional -judge flag.
//
// Usage:
//
//	memgym-score -in baseline-results.json -run run1 -out run1-scorecard.json
//	memgym-score -in results.json -run run1 -judge verdict.json -out scorecard.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"manifold/internal/agent/memory/gym"
)

func main() {
	in := flag.String("in", "", "path to gym SuiteResult JSON (required)")
	run := flag.String("run", "", "run identifier, e.g. run1 (required)")
	out := flag.String("out", "", "output scorecard path (default: stdout)")
	judge := flag.String("judge", "", "optional path to a JudgeVerdict JSON to attach")
	flag.Parse()

	if *in == "" || *run == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := score(*in, *run, *out, *judge); err != nil {
		fmt.Fprintf(os.Stderr, "memgym-score: %v\n", err)
		os.Exit(1)
	}
}

func score(in, run, out, judgePath string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return fmt.Errorf("read suite result: %w", err)
	}
	var suite gym.SuiteResult
	if err := json.Unmarshal(raw, &suite); err != nil {
		return fmt.Errorf("parse suite result: %w", err)
	}

	card := gym.BuildScorecard(run, suite)

	if judgePath != "" {
		verdictRaw, err := os.ReadFile(judgePath)
		if err != nil {
			return fmt.Errorf("read judge verdict: %w", err)
		}
		var verdict gym.JudgeVerdict
		if err := json.Unmarshal(verdictRaw, &verdict); err != nil {
			return fmt.Errorf("parse judge verdict: %w", err)
		}
		card = card.WithJudge(verdict)
	}

	payload, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal scorecard: %w", err)
	}
	payload = append(payload, '\n')

	if out == "" {
		_, err = os.Stdout.Write(payload)
		return err
	}
	if dir := filepath.Dir(out); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}
	if err := os.WriteFile(out, payload, 0o644); err != nil {
		return fmt.Errorf("write scorecard: %w", err)
	}
	return nil
}
