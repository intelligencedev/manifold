//go:build darwin

package commandexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"manifold/internal/config"
)

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
