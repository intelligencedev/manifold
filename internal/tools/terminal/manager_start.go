package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/sandbox"
)

type preparedStart struct {
	command  string
	args     []string
	base     string
	scopeKey string
	timeout  time.Duration
	rows     int
	cols     int
}

func (m *Manager) prepareStart(ctx context.Context, req StartRequest) (preparedStart, error) {
	command, args := normalizeCommandArgs(req.Command, req.Args)
	if command == "" {
		return preparedStart{}, errors.New("command is required")
	}
	if sandbox.IsBinaryBlocked(command, m.blocked) {
		return preparedStart{}, fmt.Errorf("binary is blocked or invalid: %q", command)
	}
	base, scopeKey, err := m.scope(ctx)
	if err != nil {
		return preparedStart{}, err
	}
	safeArgs, err := sanitizeTerminalArgs(base, args)
	if err != nil {
		return preparedStart{}, err
	}
	rows, cols := terminalSize(req.Rows, req.Cols)
	return preparedStart{
		command:  command,
		args:     safeArgs,
		base:     base,
		scopeKey: scopeKey,
		timeout:  m.startTimeout(req.TimeoutSeconds),
		rows:     rows,
		cols:     cols,
	}, nil
}

func sanitizeTerminalArgs(base string, args []string) ([]string, error) {
	safeArgs := make([]string, 0, len(args))
	for _, arg := range args {
		safeArg, err := sandbox.SanitizeArg(base, arg)
		if err != nil {
			return nil, err
		}
		safeArgs = append(safeArgs, safeArg)
	}
	return safeArgs, nil
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
	cmd := exec.Command(prepared.command, prepared.args...)
	cmd.Dir = prepared.base
	cmd.Env = os.Environ()
	tty, err := startProcessPTY(cmd, prepared.rows, prepared.cols)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	return &session{
		id:        id,
		name:      strings.TrimSpace(req.Name),
		scopeKey:  prepared.scopeKey,
		cwd:       prepared.base,
		command:   prepared.command,
		args:      append([]string(nil), prepared.args...),
		cmd:       cmd,
		pty:       tty,
		pid:       cmd.Process.Pid,
		startedAt: now,
		lastUsed:  now,
		buffer:    newOutputBuffer(m.bufferBytes),
		done:      make(chan struct{}),
	}, nil
}

func (m *Manager) startResult(s *session) StartResult {
	snap := s.snapshot(0, m.bufferBytes)
	return StartResult{
		OK:         true,
		TerminalID: s.id,
		Name:       s.name,
		PID:        s.pid,
		CWD:        s.cwd,
		Command:    s.command,
		Args:       append([]string(nil), s.args...),
		StartedAt:  s.startedAt,
		Running:    s.isRunning(),
		Output:     snap.Output,
		Chunks:     snap.Chunks,
		NextSeq:    snap.NextSeq,
		Truncated:  snap.Truncated,
	}
}
