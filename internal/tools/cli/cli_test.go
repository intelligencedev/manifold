package cli

import (
	"context"
	"strings"
	"testing"

	"manifold/internal/config"
)

func TestNormalizeCommandArgs(t *testing.T) {
	t.Parallel()

	command, args := normalizeCommandArgs("go version", nil)
	if command != "go" {
		t.Fatalf("command = %q, want %q", command, "go")
	}
	if len(args) != 1 || args[0] != "version" {
		t.Fatalf("args = %#v, want []string{\"version\"}", args)
	}
}

func TestNormalizeCommandArgsAppendsExplicitArgs(t *testing.T) {
	t.Parallel()

	command, args := normalizeCommandArgs("go test", []string{"./..."})
	if command != "go" {
		t.Fatalf("command = %q, want %q", command, "go")
	}
	if len(args) != 2 || args[0] != "test" || args[1] != "./..." {
		t.Fatalf("args = %#v, want []string{\"test\", \"./...\"}", args)
	}
}

func TestExecutorRunAllowsInlineCommandArgs(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(config.ExecConfig{MaxCommandSeconds: 5}, t.TempDir(), 0)
	res, err := exec.Run(context.Background(), ExecRequest{Command: "go version"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK result, got %#v", res)
	}
	if !strings.Contains(res.Stdout, "go version") {
		t.Fatalf("stdout = %q, want substring %q", res.Stdout, "go version")
	}
}

func TestExecutorRunBlocksInlineBlockedBinary(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(config.ExecConfig{MaxCommandSeconds: 5, BlockBinaries: []string{"rm"}}, t.TempDir(), 0)
	_, err := exec.Run(context.Background(), ExecRequest{Command: "rm -rf tmp"})
	if err == nil {
		t.Fatal("expected blocked binary error")
	}
	if !strings.Contains(err.Error(), "binary is blocked or invalid: \"rm\"") {
		t.Fatalf("error = %q, want blocked rm error", err.Error())
	}
}
