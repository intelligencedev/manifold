//go:build darwin

package commandexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
