package terminal

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
	"manifold/internal/sandbox"
)

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
	manager := NewManager(config.ExecConfig{MaxCommandSeconds: 5, BlockBinaries: []string{"echo"}}, t.TempDir())
	defer manager.Close()

	if _, err := manager.Start(ctx, StartRequest{}); err == nil {
		t.Fatal("expected missing command error")
	}
	if _, err := manager.Start(ctx, StartRequest{Command: "echo hi"}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked binary error, got %v", err)
	}
}

func TestStartRejectsPathTraversalArgs(t *testing.T) {
	requireCommand(t, "echo")
	ctx := testContext(t, "sess")
	manager := NewManager(config.ExecConfig{MaxCommandSeconds: 5}, t.TempDir())
	defer manager.Close()

	_, err := manager.Start(ctx, StartRequest{Command: "echo", Args: []string{"../escape"}})
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("expected path traversal error, got %v", err)
	}
}

func TestTerminalLifecycleReadWriteStop(t *testing.T) {
	requireCommand(t, "cat")
	ctx := testContext(t, "sess")
	manager := NewManager(config.ExecConfig{MaxCommandSeconds: 5}, t.TempDir())
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
	manager := NewManager(config.ExecConfig{MaxCommandSeconds: 5, MaxTerminalSessions: 2}, t.TempDir())
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
	manager := NewManager(config.ExecConfig{
		MaxCommandSeconds:         5,
		TerminalIdleTTLSeconds:    1,
		TerminalOutputBufferBytes: 1024,
	}, t.TempDir())
	defer manager.Close()

	start, err := manager.Start(ctx, StartRequest{Command: "echo", Args: []string{"done"}})
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
	manager := NewManager(config.ExecConfig{MaxCommandSeconds: 5}, base)
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
	manager := NewManager(config.ExecConfig{MaxCommandSeconds: 5, MaxTerminalRuntimeSeconds: 1}, t.TempDir())
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
	manager := NewManager(config.ExecConfig{MaxCommandSeconds: 5, TerminalOutputBufferBytes: 32}, t.TempDir())
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
