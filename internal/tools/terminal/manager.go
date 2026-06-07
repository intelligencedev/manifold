package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"manifold/internal/commandexec"
	"manifold/internal/config"
	"manifold/internal/sandbox"
)

const (
	defaultMaxTerminalSessions       = 8
	defaultTerminalIdleTTLSeconds    = 1800
	defaultTerminalOutputBufferBytes = 256 * 1024
	defaultTerminalRuntimeSeconds    = 30
	defaultInitialOutputWait         = 250 * time.Millisecond
	defaultStopGrace                 = 2 * time.Second
)

type StartRequest struct {
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	Stdin          string   `json:"stdin,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	Name           string   `json:"name,omitempty"`
	Cols           int      `json:"cols,omitempty"`
	Rows           int      `json:"rows,omitempty"`
}

type ReadRequest struct {
	TerminalID string `json:"terminal_id"`
	SinceSeq   int64  `json:"since_seq,omitempty"`
	MaxBytes   int    `json:"max_bytes,omitempty"`
}

type WriteRequest struct {
	TerminalID string `json:"terminal_id"`
	Input      string `json:"input"`
}

type StopRequest struct {
	TerminalID string `json:"terminal_id"`
	Signal     string `json:"signal,omitempty"`
}

type StartResult struct {
	OK               bool          `json:"ok"`
	Error            string        `json:"error,omitempty"`
	Decision         string        `json:"decision,omitempty"`
	PolicyID         string        `json:"policy_id,omitempty"`
	RequiresApproval bool          `json:"requires_approval,omitempty"`
	UserInstructions string        `json:"user_instructions,omitempty"`
	TerminalID       string        `json:"terminal_id,omitempty"`
	Name             string        `json:"name,omitempty"`
	PID              int           `json:"pid,omitempty"`
	CWD              string        `json:"cwd,omitempty"`
	Command          string        `json:"command,omitempty"`
	Args             []string      `json:"args,omitempty"`
	StartedAt        time.Time     `json:"started_at,omitempty"`
	Running          bool          `json:"running"`
	Output           string        `json:"output,omitempty"`
	Chunks           []outputChunk `json:"chunks,omitempty"`
	NextSeq          int64         `json:"next_seq,omitempty"`
	Truncated        bool          `json:"truncated,omitempty"`
}

type ReadResult struct {
	OK         bool          `json:"ok"`
	Error      string        `json:"error,omitempty"`
	TerminalID string        `json:"terminal_id,omitempty"`
	Name       string        `json:"name,omitempty"`
	Running    bool          `json:"running"`
	ExitCode   *int          `json:"exit_code,omitempty"`
	StartedAt  time.Time     `json:"started_at,omitempty"`
	EndedAt    *time.Time    `json:"ended_at,omitempty"`
	Output     string        `json:"output,omitempty"`
	Chunks     []outputChunk `json:"chunks,omitempty"`
	NextSeq    int64         `json:"next_seq,omitempty"`
	Truncated  bool          `json:"truncated,omitempty"`
}

type WriteResult struct {
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	TerminalID   string `json:"terminal_id,omitempty"`
	BytesWritten int    `json:"bytes_written,omitempty"`
}

type StopResult struct {
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	TerminalID string `json:"terminal_id,omitempty"`
	Stopped    bool   `json:"stopped"`
	Running    bool   `json:"running"`
	ExitCode   *int   `json:"exit_code,omitempty"`
}

type ListResult struct {
	OK        bool          `json:"ok"`
	Error     string        `json:"error,omitempty"`
	Terminals []SessionInfo `json:"terminals"`
}

type SessionInfo struct {
	TerminalID string     `json:"terminal_id"`
	Name       string     `json:"name,omitempty"`
	PID        int        `json:"pid,omitempty"`
	CWD        string     `json:"cwd"`
	Command    string     `json:"command"`
	Args       []string   `json:"args,omitempty"`
	Running    bool       `json:"running"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	NextSeq    int64      `json:"next_seq"`
}

type session struct {
	mu               sync.Mutex
	id               string
	name             string
	scopeKey         string
	cwd              string
	command          string
	args             []string
	decision         string
	policyID         string
	userInstructions string
	sandboxed        bool
	cleanup          func()
	cmd              *exec.Cmd
	pty              *os.File
	pid              int
	startedAt        time.Time
	endedAt          *time.Time
	lastUsed         time.Time
	exitCode         *int
	exitError        string
	buffer           *outputBuffer
	done             chan struct{}
	stopOnce         sync.Once
	cleanupOnce      sync.Once
	timer            *time.Timer
}

type Manager struct {
	mu      sync.Mutex
	cfgMu   sync.RWMutex
	cfg     config.ExecConfig
	workdir string

	maxSessions int
	maxRuntime  time.Duration
	idleTTL     time.Duration
	bufferBytes int

	sessions map[string]*session
	closeCh  chan struct{}
	closed   bool
}

func NewManager(cfg config.ExecConfig, workdir string) *Manager {
	cfg = withDefaults(cfg)
	m := &Manager{
		cfg:         cfg,
		workdir:     workdir,
		maxSessions: cfg.MaxTerminalSessions,
		maxRuntime:  time.Duration(cfg.MaxTerminalRuntimeSeconds) * time.Second,
		idleTTL:     time.Duration(cfg.TerminalIdleTTLSeconds) * time.Second,
		bufferBytes: cfg.TerminalOutputBufferBytes,
		sessions:    make(map[string]*session),
		closeCh:     make(chan struct{}),
	}
	go m.reapLoop()
	return m
}

func (m *Manager) AddCommandRule(rule config.ExecCommandRule) {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	m.cfg.CommandRules = append(m.cfg.CommandRules, cloneCommandRule(rule))
}

func (m *Manager) configSnapshot() config.ExecConfig {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return cloneExecConfig(m.cfg)
}

func withDefaults(cfg config.ExecConfig) config.ExecConfig {
	config.ApplyExecDefaults(&cfg)
	return cfg
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

func normalizeCommandArgs(command string, args []string) (string, []string) {
	cmd, normalizedArgs, err := commandexec.NormalizeCommandArgs(command, args)
	if err != nil {
		return "", args
	}
	return cmd, normalizedArgs
}

func (m *Manager) Start(ctx context.Context, req StartRequest) (StartResult, error) {
	prepared, err := m.prepareStart(ctx, req)
	if err != nil {
		return StartResult{}, err
	}
	cleanupPrepared := true
	defer func() {
		if cleanupPrepared && prepared.cleanup != nil {
			prepared.cleanup()
		}
	}()

	now := time.Now().UTC()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return StartResult{}, errors.New("terminal manager is closed")
	}
	m.purgeExpiredLocked(now)
	if m.runningCountLocked(prepared.scopeKey) >= m.maxSessions {
		m.mu.Unlock()
		return StartResult{}, errors.New("terminal session limit reached")
	}

	session, err := m.startSessionLocked(prepared, req, now)
	if err != nil {
		m.mu.Unlock()
		return StartResult{}, err
	}
	m.sessions[session.id] = session
	cleanupPrepared = false
	m.mu.Unlock()

	if req.Stdin != "" {
		_, _ = session.pty.Write([]byte(req.Stdin))
	}
	m.watchSession(session, prepared.timeout)
	select {
	case <-session.done:
	case <-time.After(defaultInitialOutputWait):
	}
	return m.startResult(session), nil
}

func (m *Manager) Read(ctx context.Context, req ReadRequest) (ReadResult, error) {
	s, err := m.sessionForContext(ctx, req.TerminalID)
	if err != nil {
		return ReadResult{}, err
	}
	snap := s.snapshot(req.SinceSeq, req.MaxBytes)
	info := s.info()
	return ReadResult{
		OK:         true,
		TerminalID: s.id,
		Name:       s.name,
		Running:    info.Running,
		ExitCode:   info.ExitCode,
		StartedAt:  info.StartedAt,
		EndedAt:    info.EndedAt,
		Output:     snap.Output,
		Chunks:     snap.Chunks,
		NextSeq:    snap.NextSeq,
		Truncated:  snap.Truncated,
	}, nil
}

func (m *Manager) Write(ctx context.Context, req WriteRequest) (WriteResult, error) {
	if req.Input == "" {
		return WriteResult{}, errors.New("input is required")
	}
	s, err := m.sessionForContext(ctx, req.TerminalID)
	if err != nil {
		return WriteResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endedAt != nil {
		return WriteResult{}, errors.New("terminal has exited")
	}
	n, err := s.pty.Write([]byte(req.Input))
	if err != nil {
		return WriteResult{}, err
	}
	s.lastUsed = time.Now().UTC()
	return WriteResult{OK: true, TerminalID: s.id, BytesWritten: n}, nil
}

func (m *Manager) Stop(ctx context.Context, req StopRequest) (StopResult, error) {
	s, err := m.sessionForContext(ctx, req.TerminalID)
	if err != nil {
		return StopResult{}, err
	}
	if !s.isRunning() {
		info := s.info()
		return StopResult{OK: true, TerminalID: s.id, Stopped: false, Running: false, ExitCode: info.ExitCode}, nil
	}
	if err := m.stopSession(s, req.Signal, defaultStopGrace); err != nil {
		return StopResult{}, err
	}
	info := s.info()
	return StopResult{OK: true, TerminalID: s.id, Stopped: true, Running: info.Running, ExitCode: info.ExitCode}, nil
}

func (m *Manager) List(ctx context.Context) (ListResult, error) {
	_, scopeKey, err := m.scope(ctx)
	if err != nil {
		return ListResult{}, err
	}
	now := time.Now().UTC()
	m.mu.Lock()
	m.purgeExpiredLocked(now)
	sessions := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if s.scopeKey == scopeKey {
			sessions = append(sessions, s)
			s.touch(now)
		}
	}
	m.mu.Unlock()

	out := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.info())
	}
	return ListResult{OK: true, Terminals: out}, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	close(m.closeCh)
	sessions := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*session)
	m.mu.Unlock()

	var firstErr error
	for _, s := range sessions {
		if s.isRunning() {
			if err := m.stopSession(s, "kill", 0); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if s.pty != nil {
			_ = s.pty.Close()
		}
		s.cleanupResources()
	}
	return firstErr
}

func (m *Manager) scope(ctx context.Context) (string, string, error) {
	base := sandbox.ResolveBaseDir(ctx, m.workdir)
	if strings.TrimSpace(base) == "" {
		return "", "", errors.New("workdir is required")
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", "", fmt.Errorf("resolve workdir: %w", err)
	}
	scope := absBase
	if sessionID, ok := sandbox.SessionIDFromContext(ctx); ok {
		scope += "\x00" + sessionID
	}
	return absBase, scope, nil
}

func (m *Manager) sessionForContext(ctx context.Context, id string) (*session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("terminal_id is required")
	}
	_, scopeKey, err := m.scope(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	m.mu.Lock()
	m.purgeExpiredLocked(now)
	s := m.sessions[id]
	if s == nil || s.scopeKey != scopeKey {
		m.mu.Unlock()
		return nil, errors.New("terminal not found")
	}
	s.touch(now)
	m.mu.Unlock()
	return s, nil
}

func (m *Manager) runningCountLocked(scopeKey string) int {
	count := 0
	for _, s := range m.sessions {
		if s.scopeKey == scopeKey && s.isRunning() {
			count++
		}
	}
	return count
}

func (m *Manager) purgeExpiredLocked(now time.Time) {
	for id, s := range m.sessions {
		if s.isRunning() {
			continue
		}
		if now.Sub(s.lastUsedAt()) > m.idleTTL {
			delete(m.sessions, id)
			if s.pty != nil {
				_ = s.pty.Close()
			}
			s.cleanupResources()
		}
	}
}

func (m *Manager) reapLoop() {
	interval := m.idleTTL / 2
	if interval <= 0 || interval > time.Minute {
		interval = time.Minute
	}
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.closeCh:
			return
		case <-ticker.C:
			now := time.Now().UTC()
			m.mu.Lock()
			m.purgeExpiredLocked(now)
			m.mu.Unlock()
		}
	}
}

func (m *Manager) watchSession(s *session, timeout time.Duration) {
	s.timer = time.AfterFunc(timeout, func() {
		s.mu.Lock()
		s.exitError = "terminal runtime exceeded"
		s.mu.Unlock()
		_ = m.stopSession(s, "term", defaultStopGrace)
	})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.pty.Read(buf)
			if n > 0 {
				s.appendOutput(buf[:n])
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					s.mu.Lock()
					if s.exitError == "" {
						s.exitError = err.Error()
					}
					s.mu.Unlock()
				}
				return
			}
		}
	}()
	go func() {
		err := s.cmd.Wait()
		if s.timer != nil {
			s.timer.Stop()
		}
		now := time.Now().UTC()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		s.mu.Lock()
		s.exitCode = &exitCode
		s.endedAt = &now
		s.lastUsed = now
		if err != nil && s.exitError == "" {
			s.exitError = err.Error()
		}
		s.mu.Unlock()
		_ = s.pty.Close()
		s.cleanupResources()
		close(s.done)
	}()
}

func (s *session) cleanupResources() {
	s.cleanupOnce.Do(func() {
		if s.cleanup != nil {
			s.cleanup()
		}
	})
}

func (m *Manager) stopSession(s *session, signalName string, grace time.Duration) error {
	var err error
	s.stopOnce.Do(func() {
		err = terminateProcess(s.cmd, signalName, grace, s.done)
	})
	return err
}

func (s *session) appendOutput(data []byte) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer.append(data, now)
}

func (s *session) snapshot(sinceSeq int64, maxBytes int) outputSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now().UTC()
	return s.buffer.snapshot(sinceSeq, maxBytes)
}

func (s *session) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.endedAt == nil
}

func (s *session) touch(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = now
}

func (s *session) lastUsedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUsed
}

func (s *session) info() SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionInfo{
		TerminalID: s.id,
		Name:       s.name,
		PID:        s.pid,
		CWD:        s.cwd,
		Command:    s.command,
		Args:       append([]string(nil), s.args...),
		Running:    s.endedAt == nil,
		ExitCode:   cloneIntPtr(s.exitCode),
		StartedAt:  s.startedAt,
		EndedAt:    cloneTimePtr(s.endedAt),
		NextSeq:    s.buffer.nextSeq,
	}
}

func cloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}
