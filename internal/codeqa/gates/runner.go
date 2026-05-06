package gates

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"manifold/internal/codeqa"
	"manifold/internal/codeqa/lang"
	"manifold/internal/codeqa/workspace"
)

type Gate interface {
	Name() string
	Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error)
}

type Runner struct {
	runner      codeqa.CommandRunner
	parallelism int
	factory     *workspace.Factory
	gates       []Gate
}

func NewRunner(runner codeqa.CommandRunner, parallelism int, factory *workspace.Factory, gates ...Gate) *Runner {
	return &Runner{runner: runner, parallelism: parallelism, factory: factory, gates: gates}
}

func DefaultGoGates() []Gate {
	return []Gate{NewGoFmtGate(), NewGoBuildGate(), NewGoVetGate(), NewGoTestGate()}
}

func GatesForLanguages(languages []lang.Language) []Gate {
	if len(languages) == 0 {
		return DefaultGoGates()
	}
	gates := make([]Gate, 0, len(languages)*4)
	seen := map[string]struct{}{}
	add := func(candidates ...Gate) {
		for _, gate := range candidates {
			if _, ok := seen[gate.Name()]; ok {
				continue
			}
			seen[gate.Name()] = struct{}{}
			gates = append(gates, gate)
		}
	}
	for _, language := range languages {
		switch language {
		case lang.Go:
			add(DefaultGoGates()...)
		case lang.Python:
			add(NewPythonRuffFormatGate(), NewPythonRuffCheckGate(), NewPythonPytestGate())
		case lang.JavaScript:
			add(NewPrettierCheckGate(), NewESLintGate(), NewNPMTestGate())
		case lang.TypeScript:
			add(NewPrettierCheckGate(), NewESLintGate(), NewTypeScriptCheckGate(), NewNPMTestGate())
		case lang.CSS:
			add(NewPrettierCheckGate(), NewStylelintGate())
		case lang.HTML:
			add(NewPrettierCheckGate(), NewHTMLValidateGate())
		case lang.Rust:
			add(NewCargoFmtGate(), NewCargoClippyGate(), NewCargoBuildGate(), NewCargoTestGate())
		}
	}
	if len(gates) == 0 {
		return DefaultGoGates()
	}
	return gates
}

func (r *Runner) Evaluate(ctx context.Context, repoRoot string, baseRef string, headRef string) ([]codeqa.GateResult, error) {
	if baseRef == "" {
		baseRef = "HEAD~1"
	}
	if headRef == "" {
		headRef = "HEAD"
	}
	tempRoot, err := os.MkdirTemp("", "codeqa-gates-")
	if err != nil {
		return nil, fmt.Errorf("create gate temp dir: %w", err)
	}
	defer os.RemoveAll(tempRoot)

	if r.factory == nil {
		return nil, fmt.Errorf("workspace factory is required")
	}
	refs := []string{baseRef, headRef}
	results := make([]codeqa.GateResult, 0, len(refs)*len(r.gates))
	for _, ref := range refs {
		prepared, err := r.factory.CheckoutRef(ctx, repoRoot, filepath.Join("gates", sanitizeRef(headRef)), ref)
		if err != nil {
			return nil, err
		}
		gateResults, err := r.evaluateRef(ctx, prepared.Path)
		cleanupErr := prepared.Cleanup()
		if err != nil {
			return nil, fmt.Errorf("evaluate gates on %s: %w", ref, err)
		}
		if cleanupErr != nil {
			return nil, cleanupErr
		}
		for _, result := range gateResults {
			result.Ref = ref
			if ref == headRef && !result.OK && !result.Skipped {
				result.HardFail = true
			}
			results = append(results, result)
		}
	}
	return results, nil
}

func (r *Runner) evaluateRef(ctx context.Context, dir string) ([]codeqa.GateResult, error) {
	parallelism := r.parallelism
	if parallelism <= 0 || parallelism > len(r.gates) {
		parallelism = len(r.gates)
	}
	results := make([]codeqa.GateResult, len(r.gates))
	errCh := make(chan error, len(r.gates))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for idx := range r.gates {
		idx := idx
		gate := r.gates[idx]
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
			defer func() { <-sem }()
			result, err := gate.Run(ctx, dir, r.runner)
			if err != nil {
				errCh <- fmt.Errorf("run gate %s: %w", gate.Name(), err)
				return
			}
			results[idx] = result
		}()
	}
	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return nil, err
	}
	return results, nil
}

func sanitizeRef(ref string) string {
	replacer := []struct{ old, new string }{{"/", "_"}, {":", "_"}, {"~", "_"}, {"^", "_"}}
	for _, item := range replacer {
		ref = strings.ReplaceAll(ref, item.old, item.new)
	}
	if ref == "" {
		return "head"
	}
	return ref
}
