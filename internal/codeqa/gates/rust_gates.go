package gates

import (
	"context"
	"time"

	"manifold/internal/codeqa"
)

type cargoFmtGate struct{}
type cargoClippyGate struct{}
type cargoBuildGate struct{}
type cargoTestGate struct{}

func NewCargoFmtGate() Gate    { return cargoFmtGate{} }
func NewCargoClippyGate() Gate { return cargoClippyGate{} }
func NewCargoBuildGate() Gate  { return cargoBuildGate{} }
func NewCargoTestGate() Gate   { return cargoTestGate{} }

func (cargoFmtGate) Name() string    { return "cargo_fmt" }
func (cargoClippyGate) Name() string { return "clippy" }
func (cargoBuildGate) Name() string  { return "cargo_build" }
func (cargoTestGate) Name() string   { return "cargo_test" }

func (cargoFmtGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("cargo"); err != nil {
		return codeqa.GateResult{Name: "cargo_fmt", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "cargo", Args: []string{"fmt", "--", "--check"}, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("cargo_fmt", res), nil
}

func (cargoClippyGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("cargo"); err != nil {
		return codeqa.GateResult{Name: "clippy", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "cargo", Args: []string{"clippy", "--quiet", "--all-targets", "--", "-D", "warnings"}, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("clippy", res), nil
}

func (cargoBuildGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("cargo"); err != nil {
		return codeqa.GateResult{Name: "cargo_build", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "cargo", Args: []string{"build", "--quiet"}, Timeout: 3 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("cargo_build", res), nil
}

func (cargoTestGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("cargo"); err != nil {
		return codeqa.GateResult{Name: "cargo_test", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "cargo", Args: []string{"test", "--quiet"}, Timeout: 3 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("cargo_test", res), nil
}
