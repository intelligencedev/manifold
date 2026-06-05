package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"manifold/internal/commandexec"
	"manifold/internal/config"
	"manifold/internal/sandbox"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

type ExecRequest struct {
	Command string
	Args    []string
	Timeout time.Duration
	Stdin   string
}

type ExecResult struct {
	OK               bool   `json:"ok"`
	ExitCode         int    `json:"exit_code"`
	Stdout           string `json:"stdout"`
	Stderr           string `json:"stderr"`
	Duration         int64  `json:"duration_ms"`
	Truncated        bool   `json:"truncated"`
	Error            string `json:"error,omitempty"`
	Decision         string `json:"decision,omitempty"`
	PolicyID         string `json:"policy_id,omitempty"`
	RequiresApproval bool   `json:"requires_approval,omitempty"`
	UserInstructions string `json:"user_instructions,omitempty"`
}

type Executor interface {
	Run(ctx context.Context, req ExecRequest) (ExecResult, error)
}

type ExecutorImpl struct {
	mu      sync.RWMutex
	cfg     config.ExecConfig
	workdir string
	// output limit in bytes
	outLimit int
}

func NewExecutor(cfg config.ExecConfig, workdir string, outLimit int) *ExecutorImpl {
	config.ApplyExecDefaults(&cfg)
	// If caller didn't provide an explicit outLimit, fall back to 64KB.
	if outLimit <= 0 {
		outLimit = 64 * 1024
	}
	return &ExecutorImpl{cfg: cfg, workdir: workdir, outLimit: outLimit}
}

func (e *ExecutorImpl) AddCommandRule(rule config.ExecCommandRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg.CommandRules = append(e.cfg.CommandRules, cloneCommandRule(rule))
}

func (e *ExecutorImpl) configSnapshot() config.ExecConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return cloneExecConfig(e.cfg)
}

func normalizeCommandArgs(command string, args []string) (string, []string) {
	cmd, normalizedArgs, err := commandexec.NormalizeCommandArgs(command, args)
	if err != nil {
		return "", args
	}
	return cmd, normalizedArgs
}

func (e *ExecutorImpl) Run(ctx context.Context, req ExecRequest) (ExecResult, error) {
	tracer := otel.Tracer("tools/cli")
	meter := otel.Meter("tools/cli")
	ctx, span := tracer.Start(ctx, "run")
	defer span.End()

	cmdCounter, _ := meter.Int64Counter("cli.commands.total")
	durHist, _ := meter.Int64Histogram("cli.command.duration.ms")

	if req.Command == "" {
		return ExecResult{}, errors.New("command is required")
	}

	cfg := e.configSnapshot()
	// Resolve dynamic base directory from context, defaulting to configured workdir
	base := sandbox.ResolveBaseDir(ctx, e.workdir)
	prepared, err := commandexec.Prepare(ctx, cfg, commandexec.PrepareRequest{
		Command: req.Command,
		Args:    req.Args,
		Workdir: base,
		Context: commandexec.ContextCLI,
	})
	if err != nil {
		return ExecResult{}, err
	}

	tout := req.Timeout
	if tout <= 0 || tout > time.Duration(cfg.MaxCommandSeconds)*time.Second {
		tout = time.Duration(cfg.MaxCommandSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, tout)
	defer cancel()

	c := exec.CommandContext(ctx, prepared.ExecCommand, prepared.ExecArgs...)
	c.Dir = prepared.Dir
	c.Env = prepared.Env
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if req.Stdin != "" {
		c.Stdin = bytes.NewBufferString(req.Stdin)
	}

	start := time.Now()
	err = c.Run()
	dur := time.Since(start)
	cmdCounter.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("command", prepared.Command), attribute.String("decision", prepared.Decision), attribute.Bool("sandboxed", prepared.Sandboxed)))
	durHist.Record(ctx, dur.Milliseconds(), otelmetric.WithAttributes(attribute.String("command", prepared.Command)))

	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			exit = 124
		} else {
			exit = 1
		}
	}
	span.SetAttributes(attribute.String("cli.command", prepared.Command), attribute.Int("cli.exit_code", exit), attribute.Int64("cli.duration_ms", dur.Milliseconds()), attribute.String("cli.policy_id", prepared.PolicyID), attribute.Bool("cli.sandboxed", prepared.Sandboxed))

	outS := stdout.String()
	errS := stderr.String()
	trunc := false
	if e.outLimit > 0 {
		if len(outS) > e.outLimit {
			outS = outS[:e.outLimit] + "\n[TRUNCATED]"
			trunc = true
		}
		if len(errS) > e.outLimit {
			errS = errS[:e.outLimit] + "\n[TRUNCATED]"
			trunc = true
		}
	}

	return ExecResult{OK: err == nil, ExitCode: exit, Stdout: outS, Stderr: errS, Duration: dur.Milliseconds(), Truncated: trunc, Decision: prepared.Decision, PolicyID: prepared.PolicyID, UserInstructions: prepared.UserInstructions}, nil
}

// Tool adapter ---------------------------------------------------------------

type tool struct{ exec Executor }

func NewTool(e Executor) *tool { return &tool{exec: e} }

func (t *tool) Name() string { return "run_cli" }

func (t *tool) JSONSchema() map[string]any { return buildSchema(t) }

func (t *tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Command        string   `json:"command"`
		Args           []string `json:"args"`
		TimeoutSeconds int      `json:"timeout_seconds"`
		Stdin          string   `json:"stdin"`
	}
	// Handle empty or nil JSON gracefully
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty arguments: command and args are required")
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := t.exec.Run(ctx, ExecRequest{
		Command: args.Command,
		Args:    args.Args,
		Timeout: time.Duration(args.TimeoutSeconds) * time.Second,
		Stdin:   args.Stdin,
	})
	if err != nil {
		var policyErr *commandexec.PolicyError
		if errors.As(err, &policyErr) {
			return ExecResult{
				OK:               false,
				ExitCode:         1,
				Stderr:           policyErr.Error(),
				Error:            policyErr.Error(),
				Decision:         policyErr.Decision,
				PolicyID:         policyErr.PolicyID,
				RequiresApproval: policyErr.RequiresApproval,
				UserInstructions: policyErr.UserInstructions,
			}, nil
		}
		return nil, err
	}
	return res, nil
}

func cloneExecConfig(cfg config.ExecConfig) config.ExecConfig {
	out := cfg
	out.BlockBinaries = append([]string(nil), cfg.BlockBinaries...)
	out.CommandRules = make([]config.ExecCommandRule, 0, len(cfg.CommandRules))
	for _, rule := range cfg.CommandRules {
		out.CommandRules = append(out.CommandRules, cloneCommandRule(rule))
	}
	out.Sandbox.Network.AllowedDomains = append([]string(nil), cfg.Sandbox.Network.AllowedDomains...)
	return out
}

func cloneCommandRule(rule config.ExecCommandRule) config.ExecCommandRule {
	out := rule
	out.Pattern = append([]string(nil), rule.Pattern...)
	out.Contexts = append([]string(nil), rule.Contexts...)
	return out
}
