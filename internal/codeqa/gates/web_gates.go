package gates

import (
	"context"
	"time"

	"manifold/internal/codeqa"
)

type stylelintGate struct{}
type htmlValidateGate struct{}

func NewStylelintGate() Gate    { return stylelintGate{} }
func NewHTMLValidateGate() Gate { return htmlValidateGate{} }

func (stylelintGate) Name() string    { return "stylelint" }
func (htmlValidateGate) Name() string { return "html_validate" }

func (stylelintGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("npx"); err != nil {
		return codeqa.GateResult{Name: "stylelint", OK: false, Stderr: err.Error()}, nil
	}
	files, err := listFilesByExt(dir, ".css")
	if err != nil {
		return codeqa.GateResult{}, err
	}
	if len(files) == 0 {
		return skippedGateResult("stylelint", "no CSS files"), nil
	}
	args := append([]string{"--no-install", "stylelint"}, files...)
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "npx", Args: args, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("stylelint", res), nil
}

func (htmlValidateGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("npx"); err != nil {
		return codeqa.GateResult{Name: "html_validate", OK: false, Stderr: err.Error()}, nil
	}
	files, err := listFilesByExt(dir, ".html", ".htm")
	if err != nil {
		return codeqa.GateResult{}, err
	}
	if len(files) == 0 {
		return skippedGateResult("html_validate", "no HTML files"), nil
	}
	args := append([]string{"--no-install", "html-validate"}, files...)
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "npx", Args: args, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("html_validate", res), nil
}
