//go:build darwin

package commandexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
)

func TestMacOSSandboxProfileDeniesByDefault(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	cfg := testExecConfig()
	cfg.Sandbox.Enabled = testBool(true)
	profile, err := macOSSandboxProfile(cfg, workdir, "/usr/bin/python3", nil)
	if err != nil {
		t.Fatalf("macOSSandboxProfile error: %v", err)
	}

	if !strings.Contains(profile, "(deny default)") {
		t.Fatalf("profile must deny by default: %s", profile)
	}
	if strings.Contains(profile, "(allow default)") {
		t.Fatalf("profile must not allow by default: %s", profile)
	}
	if !strings.Contains(profile, fmt.Sprintf("(allow file-read* (subpath %q))", workdir)) {
		t.Fatalf("profile must allow workspace reads: %s", profile)
	}
	if !strings.Contains(profile, fmt.Sprintf("(allow file-write* (subpath %q))", workdir)) {
		t.Fatalf("profile must allow workspace writes: %s", profile)
	}
}

func TestMacOSSandboxProfileAllowsConfiguredReadPaths(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	cfg := testExecConfig()
	cfg.Sandbox.Enabled = testBool(true)
	cfg.Sandbox.ReadPaths = []string{"/private/etc/ssl", "readonly-cache"}

	profile, err := macOSSandboxProfile(cfg, workdir, "/usr/bin/python3", nil)
	if err != nil {
		t.Fatalf("macOSSandboxProfile error: %v", err)
	}

	for _, path := range []string{"/private/etc/ssl", filepath.Join(workdir, "readonly-cache")} {
		if !strings.Contains(profile, fmt.Sprintf("(allow file-read* (subpath %q))", path)) {
			t.Fatalf("profile must allow configured read path %q: %s", path, profile)
		}
		if strings.Contains(profile, fmt.Sprintf("(allow file-write* (subpath %q))", path)) {
			t.Fatalf("profile must not allow configured read path writes %q: %s", path, profile)
		}
	}
}

func TestMacOSSandboxUnavailableFailsClosedEvenWhenConfiguredFailOpen(t *testing.T) {
	previous := macOSSandboxExecPath
	macOSSandboxExecPath = filepath.Join(t.TempDir(), "missing-sandbox-exec")
	t.Cleanup(func() { macOSSandboxExecPath = previous })

	cfg := testExecConfig()
	cfg.Sandbox.Enabled = testBool(true)
	cfg.Sandbox.FailIfUnavailable = testBool(false)
	_, _, sandboxed, err := wrapSandbox(cfg, t.TempDir(), "/usr/bin/true", nil, nil, nil)
	if err == nil {
		t.Fatalf("expected missing sandbox-exec to fail closed")
	}
	if sandboxed {
		t.Fatalf("missing sandbox-exec cannot report sandboxed execution")
	}
	var policyErr *PolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("expected policy error, got %T: %v", err, err)
	}
	if !strings.Contains(policyErr.Error(), "sandbox-exec is required but unavailable") {
		t.Fatalf("unexpected policy error: %v", policyErr)
	}
}

func TestMacOSSandboxBlocksWritesOutsideWorkspace(t *testing.T) {
	t.Parallel()
	if _, err := lookPath("python3", securePath()); err != nil {
		t.Skipf("python3 not available: %v", err)
	}
	outside := filepath.Join("/tmp", fmt.Sprintf("manifold-sandbox-%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(outside) })

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-python3", Decision: DecisionAllow, Pattern: []string{"python3"}})
	cfg.Sandbox.Enabled = testBool(true)
	prepared, err := Prepare(context.Background(), cfg, PrepareRequest{
		Command: "python3",
		Args: []string{
			"-c",
			"p=chr(47)+'tmp'+chr(47)+'" + filepath.Base(outside) + "'; open(p,'w').write('blocked')",
		},
		Workdir: t.TempDir(),
		Context: ContextCLI,
	})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}

	cmd := exec.CommandContext(context.Background(), prepared.ExecCommand, prepared.ExecArgs...)
	cmd.Dir = prepared.Dir
	cmd.Env = prepared.Env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatalf("expected sandboxed write outside workspace to fail")
	}
	if _, statErr := os.Stat(outside); statErr == nil {
		t.Fatalf("sandboxed command created outside file %s", outside)
	}
}

func TestMacOSSandboxBlocksRuntimeConstructedReadOutsideWorkspace(t *testing.T) {
	t.Parallel()
	if _, err := lookPath("python3", securePath()); err != nil {
		t.Skipf("python3 not available: %v", err)
	}

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-python3", Decision: DecisionAllow, Pattern: []string{"python3"}})
	cfg.Sandbox.Enabled = testBool(true)
	prepared, err := Prepare(context.Background(), cfg, PrepareRequest{
		Command: "python3",
		Args: []string{
			"-c",
			"p=chr(47)+'etc'+chr(47)+'hosts'; open(p).read()",
		},
		Workdir: t.TempDir(),
		Context: ContextCLI,
	})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}

	_, _, err = runPreparedCommand(t, prepared)
	if err == nil {
		t.Fatalf("expected sandboxed read outside workspace to fail")
	}
}

func TestMacOSSandboxAllowsReadsInsideWorkspace(t *testing.T) {
	t.Parallel()
	if _, err := lookPath("python3", securePath()); err != nil {
		t.Skipf("python3 not available: %v", err)
	}

	workdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workdir, "inside.txt"), []byte("workspace-data"), 0o600); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	cfg := testExecConfig(config.ExecCommandRule{ID: "allow-python3", Decision: DecisionAllow, Pattern: []string{"python3"}})
	cfg.Sandbox.Enabled = testBool(true)
	prepared, err := Prepare(context.Background(), cfg, PrepareRequest{
		Command: "python3",
		Args: []string{
			"-c",
			"print(open('inside.txt').read(), end='')",
		},
		Workdir: workdir,
		Context: ContextCLI,
	})
	if err != nil {
		t.Fatalf("Prepare error: %v", err)
	}

	stdout, stderr, err := runPreparedCommand(t, prepared)
	if err != nil {
		t.Fatalf("expected sandboxed read inside workspace to succeed: %v; stderr=%s", err, stderr)
	}
	if stdout != "workspace-data" {
		t.Fatalf("stdout = %q, want workspace-data; stderr=%s", stdout, stderr)
	}
}

func TestMacOSSandboxProfileDomainLimitedNetworkOnlyAllowsBroker(t *testing.T) {
	t.Parallel()
	cfg := testExecConfig()
	cfg.Sandbox.Enabled = testBool(true)
	cfg.Sandbox.Network.Enabled = testBool(true)
	cfg.Sandbox.Network.AllowedDomains = []string{"example.com"}
	profile, err := macOSSandboxProfile(cfg, t.TempDir(), "/usr/bin/curl", &sandboxNetwork{
		mode:     networkModeDomainLimited,
		proxyURL: "http://localhost:3128",
	})
	if err != nil {
		t.Fatalf("macOSSandboxProfile error: %v", err)
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Fatalf("domain-limited profile must deny general network: %s", profile)
	}
	if !strings.Contains(profile, `localhost:3128`) {
		t.Fatalf("domain-limited profile must allow broker endpoint: %s", profile)
	}
	if strings.Contains(profile, "(allow network*)") {
		t.Fatalf("domain-limited profile must not allow all network: %s", profile)
	}
}

func runPreparedCommand(t *testing.T, prepared PreparedCommand) (string, string, error) {
	t.Helper()
	if prepared.Cleanup != nil {
		t.Cleanup(prepared.Cleanup)
	}

	cmd := exec.CommandContext(context.Background(), prepared.ExecCommand, prepared.ExecArgs...)
	cmd.Dir = prepared.Dir
	cmd.Env = prepared.Env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
