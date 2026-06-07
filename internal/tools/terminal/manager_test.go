package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"manifold/internal/agent/inputrequest"
	"manifold/internal/commandexec"
	"manifold/internal/config"
	"manifold/internal/durable"
	"manifold/internal/sandbox"
)

func testBool(v bool) *bool {
	return &v
}

func managerTestConfig(base config.ExecConfig, commands ...string) config.ExecConfig {
	if base.MaxCommandSeconds <= 0 {
		base.MaxCommandSeconds = 5
	}
	base.Sandbox = config.ExecSandboxConfig{
		Enabled:           testBool(false),
		FailIfUnavailable: testBool(true),
		Network:           config.ExecSandboxNetworkConfig{Enabled: testBool(false)},
	}
	base.CommandRules = nil
	for _, command := range commands {
		base.CommandRules = append(base.CommandRules, config.ExecCommandRule{
			ID:       "allow-" + command,
			Decision: "allow",
			Pattern:  []string{command},
		})
	}
	return base
}

func testContext(t *testing.T, sessionID string) context.Context {
	t.Helper()
	ctx := sandbox.WithBaseDir(context.Background(), t.TempDir())
	if sessionID != "" {
		ctx = sandbox.WithSessionID(ctx, sessionID)
	}
	return ctx
}

func requireCommand(t *testing.T, command string) {
	t.Helper()
	if _, err := exec.LookPath(command); err != nil {
		t.Skipf("%s not available: %v", command, err)
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition was not met before deadline")
}

func TestStartRejectsInvalidCommands(t *testing.T) {
	requireCommand(t, "echo")
	ctx := testContext(t, "sess")
	cfg := managerTestConfig(config.ExecConfig{BlockBinaries: []string{"echo"}}, "echo")
	manager := NewManager(cfg, t.TempDir())
	defer manager.Close()

	if _, err := manager.Start(ctx, StartRequest{}); err == nil {
		t.Fatal("expected missing command error")
	}
	if _, err := manager.Start(ctx, StartRequest{Command: "echo hi"}); err == nil || !strings.Contains(err.Error(), "denied by policy") {
		t.Fatalf("expected blocked binary error, got %v", err)
	}
}

func TestStartRejectsPathTraversalArgs(t *testing.T) {
	requireCommand(t, "echo")
	ctx := testContext(t, "sess")
	manager := NewManager(managerTestConfig(config.ExecConfig{}, "echo"), t.TempDir())
	defer manager.Close()

	_, err := manager.Start(ctx, StartRequest{Command: "echo", Args: []string{"../escape"}})
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("expected path traversal error, got %v", err)
	}
}

func TestTerminalLifecycleReadWriteStop(t *testing.T) {
	requireCommand(t, "cat")
	ctx := testContext(t, "sess")
	manager := NewManager(managerTestConfig(config.ExecConfig{}, "cat"), t.TempDir())
	defer manager.Close()

	start, err := manager.Start(ctx, StartRequest{Command: "cat", Name: "interactive"})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if !start.OK || start.TerminalID == "" || !start.Running {
		t.Fatalf("unexpected start result: %#v", start)
	}

	if _, err := manager.Write(ctx, WriteRequest{TerminalID: start.TerminalID, Input: "hello terminal\n"}); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	waitUntil(t, func() bool {
		read, err := manager.Read(ctx, ReadRequest{TerminalID: start.TerminalID})
		return err == nil && strings.Contains(read.Output, "hello terminal")
	})

	stop, err := manager.Stop(ctx, StopRequest{TerminalID: start.TerminalID})
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	if !stop.OK || !stop.Stopped {
		t.Fatalf("unexpected stop result: %#v", stop)
	}
}

func TestMultipleConcurrentTerminalsAndSessionLimit(t *testing.T) {
	requireCommand(t, "cat")
	ctx := testContext(t, "sess")
	manager := NewManager(managerTestConfig(config.ExecConfig{MaxTerminalSessions: 2}, "cat"), t.TempDir())
	defer manager.Close()

	first, err := manager.Start(ctx, StartRequest{Command: "cat", Name: "one"})
	if err != nil {
		t.Fatalf("start first: %v", err)
	}
	second, err := manager.Start(ctx, StartRequest{Command: "cat", Name: "two"})
	if err != nil {
		t.Fatalf("start second: %v", err)
	}
	if _, err := manager.Start(ctx, StartRequest{Command: "cat", Name: "three"}); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected session limit error, got %v", err)
	}

	list, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(list.Terminals) != 2 {
		t.Fatalf("expected two terminals, got %#v", list.Terminals)
	}
	_, _ = manager.Stop(ctx, StopRequest{TerminalID: first.TerminalID})
	_, _ = manager.Stop(ctx, StopRequest{TerminalID: second.TerminalID})
}

func TestFinishedSessionRemainsReadableAndExpires(t *testing.T) {
	requireCommand(t, "echo")
	ctx := testContext(t, "sess")
	manager := NewManager(managerTestConfig(config.ExecConfig{
		TerminalIdleTTLSeconds:    1,
		TerminalOutputBufferBytes: 1024,
	}, "echo"), t.TempDir())
	defer manager.Close()

	start, err := manager.Start(ctx, StartRequest{Command: "echo", Args: []string{"done"}})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	waitUntil(t, func() bool {
		read, err := manager.Read(ctx, ReadRequest{TerminalID: start.TerminalID})
		return err == nil && !read.Running && strings.Contains(read.Output, "done")
	})
	read, err := manager.Read(ctx, ReadRequest{TerminalID: start.TerminalID})
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if !strings.Contains(read.Output, "done") {
		t.Fatalf("expected output to contain done, got %q", read.Output)
	}
	if read.ExitCode == nil || *read.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %#v", read.ExitCode)
	}

	time.Sleep(1100 * time.Millisecond)
	list, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(list.Terminals) != 0 {
		t.Fatalf("expected expired session to be removed, got %#v", list.Terminals)
	}
}

func TestTerminalScopeIsolation(t *testing.T) {
	requireCommand(t, "cat")
	base := t.TempDir()
	ctxA := sandbox.WithSessionID(sandbox.WithBaseDir(context.Background(), base), "a")
	ctxB := sandbox.WithSessionID(sandbox.WithBaseDir(context.Background(), base), "b")
	manager := NewManager(managerTestConfig(config.ExecConfig{}, "cat"), base)
	defer manager.Close()

	start, err := manager.Start(ctxA, StartRequest{Command: "cat"})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if _, err := manager.Read(ctxB, ReadRequest{TerminalID: start.TerminalID}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected cross-session terminal lookup to fail, got %v", err)
	}
	_, _ = manager.Stop(ctxA, StopRequest{TerminalID: start.TerminalID})
}

func TestRuntimeTimeoutStopsTerminal(t *testing.T) {
	requireCommand(t, "sleep")
	ctx := testContext(t, "sess")
	manager := NewManager(managerTestConfig(config.ExecConfig{MaxTerminalRuntimeSeconds: 1}, "sleep"), t.TempDir())
	defer manager.Close()

	start, err := manager.Start(ctx, StartRequest{Command: "sleep", Args: []string{"10"}, TimeoutSeconds: 1})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	waitUntil(t, func() bool {
		read, err := manager.Read(ctx, ReadRequest{TerminalID: start.TerminalID})
		return err == nil && !read.Running
	})
}

func TestOutputBufferTruncates(t *testing.T) {
	requireCommand(t, "echo")
	ctx := testContext(t, "sess")
	manager := NewManager(managerTestConfig(config.ExecConfig{TerminalOutputBufferBytes: 32}, "echo"), t.TempDir())
	defer manager.Close()

	start, err := manager.Start(ctx, StartRequest{Command: "echo", Args: []string{strings.Repeat("x", 80)}})
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	waitUntil(t, func() bool {
		read, err := manager.Read(ctx, ReadRequest{TerminalID: start.TerminalID})
		return err == nil && !read.Running
	})
	read, err := manager.Read(ctx, ReadRequest{TerminalID: start.TerminalID})
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if !read.Truncated {
		t.Fatalf("expected truncated output, got %#v", read)
	}
	if len(read.Output) > 32 {
		t.Fatalf("expected output to be capped, len=%d output=%q", len(read.Output), read.Output)
	}
}

func TestTerminalStartToolReturnsStablePolicyPayload(t *testing.T) {
	t.Parallel()

	manager := NewManager(managerTestConfig(config.ExecConfig{}, "echo"), t.TempDir())
	defer manager.Close()

	payload, err := NewStartTool(manager).Call(testContext(t, "sess"), json.RawMessage(`{"command":"python --version"}`))
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	res, ok := payload.(StartResult)
	if !ok {
		t.Fatalf("payload type = %T, want StartResult", payload)
	}
	if res.OK || res.Decision != "deny" || res.Error == "" {
		t.Fatalf("unexpected policy payload: %#v", res)
	}
}

func TestTerminalStartToolPropagatesDurableSuspension(t *testing.T) {
	t.Parallel()

	cfg := managerTestConfig(config.ExecConfig{})
	cfg.CommandRules = []config.ExecCommandRule{{
		ID:       "ask-echo",
		Decision: "ask",
		Pattern:  []string{"echo"},
	}}
	manager := NewManager(cfg, t.TempDir())
	defer manager.Close()

	ctx := testContext(t, "sess")
	ctx = inputrequest.WithRequester(ctx, suspendingInputRequester{})
	ctx = commandexec.WithApprovalController(ctx, testApprovalController{})

	payload, err := NewStartTool(manager).Call(ctx, json.RawMessage(`{"command":"echo","args":["hi"]}`))
	if !errors.Is(err, durable.ErrSuspended) {
		t.Fatalf("Call error = %v, want durable suspension with nil payload", err)
	}
	if payload != nil {
		t.Fatalf("payload = %#v, want nil", payload)
	}
}

type suspendingInputRequester struct{}

func (suspendingInputRequester) RequestInfo(context.Context, inputrequest.Request) (inputrequest.Response, error) {
	return inputrequest.Response{}, durable.ErrSuspended
}

type testApprovalController struct{}

func (testApprovalController) PersistCommandAllowRule(_ context.Context, rule config.ExecCommandRule) (config.ExecCommandRule, error) {
	return rule, nil
}

func (testApprovalController) SessionAllowAllCommands(context.Context, commandexec.CommandSessionScope) (bool, error) {
	return false, nil
}

func (testApprovalController) SetSessionAllowAllCommands(context.Context, commandexec.CommandSessionScope, bool) error {
	return nil
}
