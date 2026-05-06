package codeqa

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"manifold/internal/sandbox"
	"manifold/internal/tools/cli"
)

type CommandRequest struct {
	Command string
	Args    []string
	Timeout time.Duration
	Stdin   string
}

type CommandResult struct {
	OK         bool
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMs int64
	Truncated  bool
}

type CommandRunner interface {
	Run(ctx context.Context, dir string, req CommandRequest) (CommandResult, error)
	LookPath(file string) (string, error)
}

type CLICommandRunner struct {
	exec    cli.Executor
	allowed map[string]struct{}
}

func NewCLICommandRunner(executor cli.Executor, allowedCommands []string) *CLICommandRunner {
	allowed := make(map[string]struct{}, len(allowedCommands)+1)
	for _, command := range allowedCommands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		allowed[command] = struct{}{}
	}
	allowed["git"] = struct{}{}
	return &CLICommandRunner{exec: executor, allowed: allowed}
}

func (r *CLICommandRunner) LookPath(file string) (string, error) {
	if !r.isAllowed(file) {
		return "", fmt.Errorf("command %q is not allowed", file)
	}
	return exec.LookPath(file)
}

func (r *CLICommandRunner) Run(ctx context.Context, dir string, req CommandRequest) (CommandResult, error) {
	if !r.isAllowed(req.Command) {
		return CommandResult{}, fmt.Errorf("command %q is not allowed", req.Command)
	}
	ctx = sandbox.WithBaseDir(ctx, dir)
	res, err := r.exec.Run(ctx, cli.ExecRequest{
		Command: req.Command,
		Args:    req.Args,
		Timeout: req.Timeout,
		Stdin:   req.Stdin,
	})
	if err != nil {
		return CommandResult{}, err
	}
	return CommandResult{
		OK:         res.OK,
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		Stderr:     res.Stderr,
		DurationMs: res.Duration,
		Truncated:  res.Truncated,
	}, nil
}

func (r *CLICommandRunner) isAllowed(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	_, ok := r.allowed[command]
	return ok
}
