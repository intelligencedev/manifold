package terminal

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/commandexec"
)

type preparedStart struct {
	command          string
	args             []string
	execCommand      string
	execArgs         []string
	env              []string
	base             string
	scopeKey         string
	timeout          time.Duration
	rows             int
	cols             int
	decision         string
	policyID         string
	userInstructions string
	sandboxed        bool
	cleanup          func()
}

func (m *Manager) prepareStart(ctx context.Context, req StartRequest) (preparedStart, error) {
	if strings.TrimSpace(req.Command) == "" {
		return preparedStart{}, errors.New("command is required")
	}
	base, scopeKey, err := m.scope(ctx)
	if err != nil {
		return preparedStart{}, err
	}
	cfg := m.configSnapshot()
	prepared, err := commandexec.Prepare(ctx, cfg, commandexec.PrepareRequest{
		Command: req.Command,
		Args:    req.Args,
		Workdir: base,
		Context: commandexec.ContextTerminal,
	})
	if err != nil {
		return preparedStart{}, err
	}
	rows, cols := terminalSize(req.Rows, req.Cols)
	return preparedStart{
		command:          prepared.Command,
		args:             prepared.Args,
		execCommand:      prepared.ExecCommand,
		execArgs:         prepared.ExecArgs,
		env:              prepared.Env,
		base:             prepared.Dir,
		scopeKey:         scopeKey,
		timeout:          m.startTimeout(req.TimeoutSeconds),
		rows:             rows,
		cols:             cols,
		decision:         prepared.Decision,
		policyID:         prepared.PolicyID,
		userInstructions: prepared.UserInstructions,
		sandboxed:        prepared.Sandboxed,
		cleanup:          prepared.Cleanup,
	}, nil
}

func terminalSize(rows, cols int) (int, int) {
	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}
	return rows, cols
}

func (m *Manager) startTimeout(seconds int) time.Duration {
	timeout := time.Duration(seconds) * time.Second
	if timeout <= 0 || timeout > m.maxRuntime {
		return m.maxRuntime
	}
	return timeout
}

func (m *Manager) startSessionLocked(prepared preparedStart, req StartRequest, now time.Time) (*session, error) {
	cmd := exec.Command(prepared.execCommand, prepared.execArgs...)
	cmd.Dir = prepared.base
	cmd.Env = prepared.env
	tty, err := startProcessPTY(cmd, prepared.rows, prepared.cols)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	return &session{
		id:               id,
		name:             strings.TrimSpace(req.Name),
		scopeKey:         prepared.scopeKey,
		cwd:              prepared.base,
		command:          prepared.command,
		args:             append([]string(nil), prepared.args...),
		decision:         prepared.decision,
		policyID:         prepared.policyID,
		userInstructions: prepared.userInstructions,
		sandboxed:        prepared.sandboxed,
		cleanup:          prepared.cleanup,
		cmd:              cmd,
		pty:              tty,
		pid:              cmd.Process.Pid,
		startedAt:        now,
		lastUsed:         now,
		buffer:           newOutputBuffer(m.bufferBytes),
		done:             make(chan struct{}),
	}, nil
}

func (m *Manager) startResult(s *session) StartResult {
	snap := s.snapshot(0, m.bufferBytes)
	return StartResult{
		OK:               true,
		Decision:         s.decision,
		PolicyID:         s.policyID,
		UserInstructions: s.userInstructions,
		TerminalID:       s.id,
		Name:             s.name,
		PID:              s.pid,
		CWD:              s.cwd,
		Command:          s.command,
		Args:             append([]string(nil), s.args...),
		StartedAt:        s.startedAt,
		Running:          s.isRunning(),
		Output:           snap.Output,
		Chunks:           snap.Chunks,
		NextSeq:          snap.NextSeq,
		Truncated:        snap.Truncated,
	}
}
