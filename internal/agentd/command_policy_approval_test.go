package agentd

import (
	"context"
	"os"
	"strings"
	"testing"

	"manifold/internal/config"
	"manifold/internal/tools/cli"
	terminaltool "manifold/internal/tools/terminal"
)

func TestCommandPolicyApprovalPersistsAndUpdatesLivePolicy(t *testing.T) {
	dir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir error: %v", err)
	}
	if err := os.WriteFile("config.yaml", []byte("exec:\n  commandRules:\n    - id: allow-go\n      decision: allow\n      pattern: [go]\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	cfg := &config.Config{
		Workdir: dir,
		Exec: config.ExecConfig{
			MaxCommandSeconds: 5,
			CommandRules: []config.ExecCommandRule{{
				ID:       "allow-go",
				Decision: "allow",
				Pattern:  []string{"go"},
			}},
			Sandbox: config.ExecSandboxConfig{
				Enabled:           commandPolicyTestBool(false),
				FailIfUnavailable: commandPolicyTestBool(true),
				Network:           config.ExecSandboxNetworkConfig{Enabled: commandPolicyTestBool(false)},
			},
		},
	}
	executor := cli.NewExecutor(cfg.Exec, dir, 0)
	terminalManager := terminaltool.NewManager(cfg.Exec, dir)
	t.Cleanup(func() {
		_ = terminalManager.Close()
	})
	a := &app{cfg: cfg, cliExecutor: executor, terminalManager: terminalManager}

	rule := config.ExecCommandRule{
		ID:            "approved:cli:test",
		Decision:      "allow",
		Pattern:       []string{"echo", "hi"},
		Contexts:      []string{"cli"},
		Justification: "test approval",
	}
	persisted, err := commandPolicyApprovalController{app: a}.PersistCommandAllowRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("PersistCommandAllowRule error: %v", err)
	}
	if persisted.ID != rule.ID {
		t.Fatalf("persisted ID = %q, want %q", persisted.ID, rule.ID)
	}

	res, err := executor.Run(context.Background(), cli.ExecRequest{Command: "echo hi"})
	if err != nil {
		t.Fatalf("Run after persisted approval returned error: %v", err)
	}
	if !res.OK || res.PolicyID != rule.ID || !strings.Contains(res.Stdout, "hi") {
		t.Fatalf("unexpected run result after policy update: %#v", res)
	}

	b, err := os.ReadFile("config.yaml")
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if !strings.Contains(string(b), "approved:cli:test") {
		t.Fatalf("persisted config missing approval rule:\n%s", string(b))
	}
}

func commandPolicyTestBool(v bool) *bool {
	return &v
}
