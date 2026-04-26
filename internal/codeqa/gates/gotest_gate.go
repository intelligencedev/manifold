package gates

import (
	"context"
	"time"

	"manifold/internal/codeqa"
)

type goTestGate struct{}

func NewGoTestGate() Gate { return goTestGate{} }

func (goTestGate) Name() string { return "go_test" }

func (goTestGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("go"); err != nil {
		return codeqa.GateResult{Name: "go_test", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "go", Args: []string{"test", "./..."}, Timeout: 3 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return codeqa.GateResult{Name: "go_test", OK: res.OK, Stdout: res.Stdout, Stderr: res.Stderr, DurationMs: res.DurationMs}, nil
}
