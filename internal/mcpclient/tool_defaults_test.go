package mcpclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcppkg "github.com/modelcontextprotocol/go-sdk/mcp"

	"manifold/internal/sandbox"
)

func TestMCPToolApplyDefaultsUsesProjectContextAndPreservesCallerArgs(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	tool := &mcpTool{
		server:  "chrome-devtools",
		tool:    &mcppkg.Tool{Name: "take_screenshot"},
		workdir: "/fallback",
		defaults: map[string]any{
			"filePath": ".manifold/chrome-devtools/screenshots/screenshot-{{TIMESTAMP}}.png",
			"format":   "png",
		},
	}
	args := map[string]any{"format": "webp"}

	got := tool.applyDefaults(sandbox.WithBaseDir(context.Background(), projectDir), args).(map[string]any)

	if got["format"] != "webp" {
		t.Fatalf("caller format was overwritten: %#v", got)
	}
	filePath, ok := got["filePath"].(string)
	if !ok || !strings.HasPrefix(filePath, ".manifold/chrome-devtools/screenshots/screenshot-") || !strings.HasSuffix(filePath, ".png") {
		t.Fatalf("unexpected filePath default: %#v", got)
	}
	parent := filepath.Join(projectDir, ".manifold", "chrome-devtools", "screenshots")
	if !dirExists(parent) {
		t.Fatalf("expected filePath parent %q to exist", parent)
	}
}

func TestRenderMCPDefaultTemplatesLeavesUnknownRuntimeTokens(t *testing.T) {
	t.Parallel()

	got := renderMCPDefaultTemplates(
		map[string]any{"filePath": "{{PROJECT_DIR}}/screens/{{TIMESTAMP}}.png"},
		mcpDefaultTemplateData{ProjectDir: "/project"},
	)

	if got["filePath"] != "/project/screens/{{TIMESTAMP}}.png" {
		t.Fatalf("unexpected rendered template: %#v", got)
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
