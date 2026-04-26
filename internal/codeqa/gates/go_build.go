package gates

import (
	"context"
	"time"

	"manifold/internal/codeqa"
)

type goBuildGate struct{}

func NewGoBuildGate() Gate { return goBuildGate{} }

func (goBuildGate) Name() string { return "go_build" }

func (goBuildGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("go"); err != nil {
		return codeqa.GateResult{Name: "go_build", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "go", Args: []string{"build", "./..."}, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return codeqa.GateResult{Name: "go_build", OK: res.OK, Stdout: res.Stdout, Stderr: res.Stderr, DurationMs: res.DurationMs}, nil
}
