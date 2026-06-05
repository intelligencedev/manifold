package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"manifold/internal/config"
)

func testBool(v bool) *bool {
	return &v
}

func testExecConfig(rules ...config.ExecCommandRule) config.ExecConfig {
	return config.ExecConfig{
		MaxCommandSeconds: 5,
		CommandRules:      rules,
		Sandbox: config.ExecSandboxConfig{
			Enabled:           testBool(false),
			FailIfUnavailable: testBool(true),
			Network:           config.ExecSandboxNetworkConfig{Enabled: testBool(false)},
		},
	}
}

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

	exec := NewExecutor(testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: "allow", Pattern: []string{"go"}}), t.TempDir(), 0)
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

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-rm", Decision: "allow", Pattern: []string{"rm"}})
	cfg.BlockBinaries = []string{"rm"}
	exec := NewExecutor(cfg, t.TempDir(), 0)
	_, err := exec.Run(context.Background(), ExecRequest{Command: "rm -rf tmp"})
	if err == nil {
		t.Fatal("expected blocked binary error")
	}
	if !strings.Contains(err.Error(), "command is denied by policy") {
		t.Fatalf("error = %q, want blocked rm error", err.Error())
	}
}

func TestRunCLIToolReturnsStablePolicyPayload(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(testExecConfig(config.ExecCommandRule{ID: "allow-go", Decision: "allow", Pattern: []string{"go"}}), t.TempDir(), 0)
	payload, err := NewTool(exec).Call(context.Background(), json.RawMessage(`{"command":"python --version"}`))
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	res, ok := payload.(ExecResult)
	if !ok {
		t.Fatalf("payload type = %T, want ExecResult", payload)
	}
	if res.OK || res.Decision != "deny" || res.Error == "" {
		t.Fatalf("unexpected policy payload: %#v", res)
	}
}
