package gates

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"manifold/internal/codeqa"
)

func commandGateResult(name string, res codeqa.CommandResult) codeqa.GateResult {
	return codeqa.GateResult{Name: name, OK: res.OK, Stdout: res.Stdout, Stderr: res.Stderr, DurationMs: res.DurationMs}
}

func skippedGateResult(name string, reason string) codeqa.GateResult {
	return codeqa.GateResult{Name: name, OK: true, Skipped: true, Stdout: "skipped: " + reason}
}

func listFilesByExt(dir string, exts ...string) ([]string, error) {
	want := make(map[string]struct{}, len(exts))
	for _, ext := range exts {
		want[strings.ToLower(ext)] = struct{}{}
	}
	files := make([]string, 0, 64)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", "vendor", ".venv", "venv", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := want[strings.ToLower(filepath.Ext(path))]; !ok {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

func fileExists(dir string, rel string) bool {
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

func hasPythonTests(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".venv", "venv", "node_modules", "dist", "build", "vendor", "__pycache__":
				return filepath.SkipDir
			case "tests":
				found = true
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py") || strings.HasSuffix(name, "_test.py") {
			found = true
		}
		return nil
	})
	return found
}

func packageJSONHasScript(dir string, script string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	return strings.TrimSpace(pkg.Scripts[script]) != ""
}
