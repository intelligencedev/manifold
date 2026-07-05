package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeDevStartsFrontendBeforeBackend(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	output := makeDryRun(t, root, "dev")
	frontendIndex := firstIndex(output, []string{
		"dev-frontend &",
		"pnpm run dev",
	})
	if frontendIndex < 0 {
		t.Fatalf("make -n dev did not include frontend startup:\n%s", output)
	}

	backendIndex := firstIndex(output, []string{
		"dev-backend",
		"FRONTEND_DEV_PROXY=",
	})
	if backendIndex < 0 {
		t.Fatalf("make -n dev did not include backend startup:\n%s", output)
	}

	if backendIndex < frontendIndex {
		t.Fatalf("make dev starts backend before frontend; backend blocks Vite startup:\n%s", output)
	}
}

func TestMakeDevFrontendPassesVitePortToScript(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	output := makeDryRun(t, root, "dev-frontend")
	want := "pnpm run dev --host 127.0.0.1 --port 5177 --strictPort"
	if !strings.Contains(output, want) {
		t.Fatalf("make dev-frontend should pass host and port flags to Vite without an extra --\nwant: %s\noutput:\n%s", want, output)
	}
}

func makeDryRun(t *testing.T, root string, target string) string {
	t.Helper()

	cmd := exec.Command("make", "-n", "--no-print-directory", target)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n %s failed: %v\n%s", target, err, out)
	}
	return string(out)
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root with Makefile not found")
		}
		dir = parent
	}
}

func firstIndex(s string, needles []string) int {
	best := -1
	for _, needle := range needles {
		index := strings.Index(s, needle)
		if index >= 0 && (best == -1 || index < best) {
			best = index
		}
	}
	return best
}
