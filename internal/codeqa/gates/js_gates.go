package gates

import (
	"context"
	"time"

	"manifold/internal/codeqa"
)

type prettierCheckGate struct{}
type eslintGate struct{}
type typeScriptCheckGate struct{}
type npmTestGate struct{}

func NewPrettierCheckGate() Gate   { return prettierCheckGate{} }
func NewESLintGate() Gate          { return eslintGate{} }
func NewTypeScriptCheckGate() Gate { return typeScriptCheckGate{} }
func NewNPMTestGate() Gate         { return npmTestGate{} }

func (prettierCheckGate) Name() string   { return "prettier_check" }
func (eslintGate) Name() string          { return "eslint" }
func (typeScriptCheckGate) Name() string { return "tsc" }
func (npmTestGate) Name() string         { return "npm_test" }

func (prettierCheckGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("npx"); err != nil {
		return codeqa.GateResult{Name: "prettier_check", OK: false, Stderr: err.Error()}, nil
	}
	files, err := listFilesByExt(dir, ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".css", ".html", ".htm")
	if err != nil {
		return codeqa.GateResult{}, err
	}
	if len(files) == 0 {
		return skippedGateResult("prettier_check", "no Prettier-supported files"), nil
	}
	args := append([]string{"--no-install", "prettier", "--check"}, files...)
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "npx", Args: args, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("prettier_check", res), nil
}

func (eslintGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if _, err := runner.LookPath("npx"); err != nil {
		return codeqa.GateResult{Name: "eslint", OK: false, Stderr: err.Error()}, nil
	}
	files, err := listFilesByExt(dir, ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts")
	if err != nil {
		return codeqa.GateResult{}, err
	}
	if len(files) == 0 {
		return skippedGateResult("eslint", "no JavaScript or TypeScript files"), nil
	}
	args := append([]string{"--no-install", "eslint"}, files...)
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "npx", Args: args, Timeout: 2 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("eslint", res), nil
}

func (typeScriptCheckGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if !fileExists(dir, "tsconfig.json") {
		return skippedGateResult("tsc", "no tsconfig.json"), nil
	}
	if _, err := runner.LookPath("npx"); err != nil {
		return codeqa.GateResult{Name: "tsc", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "npx", Args: []string{"--no-install", "tsc", "--noEmit"}, Timeout: 3 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("tsc", res), nil
}

func (npmTestGate) Run(ctx context.Context, dir string, runner codeqa.CommandRunner) (codeqa.GateResult, error) {
	if !packageJSONHasScript(dir, "test") {
		return skippedGateResult("npm_test", "no package.json test script"), nil
	}
	if _, err := runner.LookPath("npm"); err != nil {
		return codeqa.GateResult{Name: "npm_test", OK: false, Stderr: err.Error()}, nil
	}
	res, err := runner.Run(ctx, dir, codeqa.CommandRequest{Command: "npm", Args: []string{"test", "--silent"}, Timeout: 3 * time.Minute})
	if err != nil {
		return codeqa.GateResult{}, err
	}
	return commandGateResult("npm_test", res), nil
}
