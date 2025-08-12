package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"gptagent/internal/config"
	"gptagent/internal/sandbox"

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
	OK        bool   `json:"ok"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Duration  int64  `json:"duration_ms"`
	Truncated bool   `json:"truncated"`
}

type Executor interface {
	Run(ctx context.Context, req ExecRequest) (ExecResult, error)
}

type ExecutorImpl struct {
	cfg     config.ExecConfig
	workdir string
	// derived block set
	blocked map[string]struct{}
	// output limit in bytes
	outLimit int
}

func NewExecutor(cfg config.ExecConfig, workdir string) *ExecutorImpl {
	blocked := make(map[string]struct{}, len(cfg.BlockBinaries))
	for _, b := range cfg.BlockBinaries {
		blocked[b] = struct{}{}
	}
	return &ExecutorImpl{cfg: cfg, workdir: workdir, blocked: blocked, outLimit: 64 * 1024}
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
	if sandbox.IsBinaryBlocked(req.Command, e.blocked) {
		return ExecResult{}, fmt.Errorf("binary is blocked or invalid: %q", req.Command)
	}

	safeArgs := make([]string, 0, len(req.Args))
	for _, a := range req.Args {
		s, err := sandbox.SanitizeArg(e.workdir, a)
		if err != nil {
			return ExecResult{}, err
		}
		safeArgs = append(safeArgs, s)
	}

	tout := req.Timeout
	if tout <= 0 || tout > time.Duration(e.cfg.MaxCommandSeconds)*time.Second {
		tout = time.Duration(e.cfg.MaxCommandSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, tout)
	defer cancel()

	c := exec.CommandContext(ctx, req.Command, safeArgs...)
	c.Dir = e.workdir
	c.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if req.Stdin != "" {
		c.Stdin = bytes.NewBufferString(req.Stdin)
	}

	start := time.Now()
	err := c.Run()
	dur := time.Since(start)
	cmdCounter.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("command", req.Command)))
	durHist.Record(ctx, dur.Milliseconds(), otelmetric.WithAttributes(attribute.String("command", req.Command)))

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
	span.SetAttributes(attribute.String("cli.command", req.Command), attribute.Int("cli.exit_code", exit), attribute.Int64("cli.duration_ms", dur.Milliseconds()))

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

	return ExecResult{OK: err == nil, ExitCode: exit, Stdout: outS, Stderr: errS, Duration: dur.Milliseconds(), Truncated: trunc}, nil
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
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	res, err := t.exec.Run(ctx, ExecRequest{
		Command: args.Command,
		Args:    args.Args,
		Timeout: time.Duration(args.TimeoutSeconds) * time.Second,
		Stdin:   args.Stdin,
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
