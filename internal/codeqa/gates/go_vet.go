package gates

import (
	"context"
	"time"

	"manifold/internal/codeqa"
)

type goVetGate struct{}

func NewGoVetGate() Gate { return goVetGate{} }

func (goVetGate) Name() string { return "go_vet" }

func (goVetGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("go"); err != nil {
		return codeqa.GateResult{Name: "go_vet", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "go", Args: []string{"vet", "./..."}, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return codeqa.GateResult{Name: "go_vet", OK: res.OK, Stdout: res.Stdout, Stderr: res.Stderr, DurationMs: res.DurationMs}, nil
}
