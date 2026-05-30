package lang

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFromPath(t *testing.T) {
	t.Parallel()
	tests := map[string]Language{
		"main.go":       Go,
		"app.py":        Python,
		"index.js":      JavaScript,
		"component.jsx": JavaScript,
		"main.ts":       TypeScript,
		"view.tsx":      TypeScript,
		"style.css":     CSS,
		"index.html":    HTML,
		"main.rs":       Rust,
		"README.md":     "",
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if got := FromPath(path); got != want {
				t.Fatalf("FromPath(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

func TestDetectUsesChangedFilesFirst(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeFile(t, repo, "package.json", "{}")
	got := Detect(repo, []string{"internal/app.py", "web/app.ts", "web/style.css", "crates/app/src/main.rs"})
	want := []Language{Python, TypeScript, CSS, Rust}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Detect() = %v, want %v", got, want)
	}
}

func TestDetectFallsBackToMarkers(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeFile(t, repo, "go.mod", "module example.com/repo\n")
	writeFile(t, repo, "web/package.json", "{}")
	writeFile(t, repo, "web/tsconfig.json", "{}")
	writeFile(t, repo, "pyproject.toml", "[project]\n")
	writeFile(t, repo, "Cargo.toml", "[package]\nname = \"app\"\n")
	got := Detect(repo, nil)
	want := []Language{Go, Python, JavaScript, TypeScript, Rust}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Detect() = %v, want %v", got, want)
	}
}

func writeFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
