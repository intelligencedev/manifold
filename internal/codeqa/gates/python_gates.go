package gates

import (
	"context"
	"time"

	"manifold/internal/codeqa"
)

type pythonRuffFormatGate struct{}
type pythonRuffCheckGate struct{}
type pythonPytestGate struct{}

func NewPythonRuffFormatGate() Gate { return pythonRuffFormatGate{} }
func NewPythonRuffCheckGate() Gate  { return pythonRuffCheckGate{} }
func NewPythonPytestGate() Gate     { return pythonPytestGate{} }

func (pythonRuffFormatGate) Name() string { return "ruff_format" }
func (pythonRuffCheckGate) Name() string  { return "ruff_check" }
func (pythonPytestGate) Name() string     { return "pytest" }

func (pythonRuffFormatGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("ruff"); err != nil {
		return codeqa.GateResult{Name: "ruff_format", OK: false, Stderr: err.Error()}, nil
	}
	files, err := listFilesByExt(dir, ".py", ".pyw")
	if err != nil {
		return codeqa.GateResult{}, err
	}
	if len(files) == 0 {
		return skippedGateResult("ruff_format", "no Python files"), nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "ruff", Args: append([]string{"format", "--check"}, files...), Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("ruff_format", res), nil
}

func (pythonRuffCheckGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("ruff"); err != nil {
		return codeqa.GateResult{Name: "ruff_check", OK: false, Stderr: err.Error()}, nil
	}
	files, err := listFilesByExt(dir, ".py", ".pyw")
	if err != nil {
		return codeqa.GateResult{}, err
	}
	if len(files) == 0 {
		return skippedGateResult("ruff_check", "no Python files"), nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "ruff", Args: append([]string{"check"}, files...), Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("ruff_check", res), nil
}

func (pythonPytestGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if !hasPythonTests(dir) {
		return skippedGateResult("pytest", "no Python tests found"), nil
	}
	if _, err := runner.LookPath("pytest"); err != nil {
		return codeqa.GateResult{Name: "pytest", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "pytest", Args: []string{"-q"}, Timeout: 3 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("pytest", res), nil
}
