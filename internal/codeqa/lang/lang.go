package lang

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Language string

const (
	Go         Language = "go"
	Python     Language = "python"
	JavaScript Language = "javascript"
	TypeScript Language = "typescript"
	CSS        Language = "css"
	HTML       Language = "html"
	Rust       Language = "rust"
)

var orderedLanguages = []Language{Go, Python, JavaScript, TypeScript, CSS, HTML, Rust}

func FromPath(path string) Language {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return Go
	case ".py", ".pyw":
		return Python
	case ".js", ".jsx", ".mjs", ".cjs":
		return JavaScript
	case ".ts", ".tsx", ".mts", ".cts":
		return TypeScript
	case ".css":
		return CSS
	case ".html", ".htm":
		return HTML
	case ".rs":
		return Rust
	default:
		return ""
	}
}

func Detect(repoRoot string, changedFiles []string) []Language {
	seen := map[Language]struct{}{}
	for _, file := range changedFiles {
		if language := FromPath(file); language != "" {
			seen[language] = struct{}{}
		}
	}
	if len(seen) == 0 {
		for _, language := range detectMarkers(repoRoot) {
			seen[language] = struct{}{}
		}
	}
	return sortLanguages(seen)
}

func sortLanguages(seen map[Language]struct{}) []Language {
	langs := make([]Language, 0, len(seen))
	for _, language := range orderedLanguages {
		if _, ok := seen[language]; ok {
			langs = append(langs, language)
		}
	}
	return langs
}

func detectMarkers(repoRoot string) []Language {
	seen := map[Language]struct{}{}
	if repoRoot == "" {
		return nil
	}
	_ = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build", "vendor", ".venv", "venv", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(d.Name()) {
		case "go.mod":
			seen[Go] = struct{}{}
		case "pyproject.toml", "setup.py", "requirements.txt", "poetry.lock", "pdm.lock":
			seen[Python] = struct{}{}
		case "package.json":
			seen[JavaScript] = struct{}{}
		case "tsconfig.json":
			seen[TypeScript] = struct{}{}
		case "cargo.toml":
			seen[Rust] = struct{}{}
		}
		return nil
	})
	if len(seen) == 0 {
		return nil
	}
	return sortLanguages(seen)
}

func Exists(root string, rel string) bool {
	if root == "" || rel == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

func Contains(languages []Language, target Language) bool {
	return slices.Contains(languages, target)
}
