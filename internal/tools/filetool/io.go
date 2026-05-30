package filetool

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"manifold/internal/sandbox"
)

type rootGuard struct {
	roots []string
}

func newRootGuard(roots []string) rootGuard {
	seen := make(map[string]struct{}, len(roots))
	cleaned := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			abs = filepath.Clean(root)
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		cleaned = append(cleaned, abs)
	}
	return rootGuard{roots: cleaned}
}

func (g rootGuard) baseDir(ctx context.Context) (string, error) {
	base, ok := sandbox.BaseDirFromContext(ctx)
	if !ok || strings.TrimSpace(base) == "" {
		return "", errors.New("no project base directory in context; file tools must run inside a project")
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve base directory: %w", err)
	}
	info, err := os.Stat(absBase)
	if err != nil {
		return "", fmt.Errorf("base directory unavailable: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("base directory is not a directory")
	}
	if len(g.roots) == 0 {
		return absBase, nil
	}
	for _, root := range g.roots {
		if isWithinRoot(root, absBase) {
			return absBase, nil
		}
	}
	return "", fmt.Errorf("base directory is outside allowed roots")
}

func isWithinRoot(root, candidate string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func cleanInputPath(p string) string {
	p = strings.TrimSpace(p)
	for strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		p = p[1:]
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, ".\\")
	return p
}

func resolvePath(base, input string) (string, string, error) {
	cleaned := cleanInputPath(input)
	if cleaned == "" {
		return "", "", fmt.Errorf("empty path")
	}
	candidate := cleaned
	if !strings.HasPrefix(candidate, ".") && !strings.ContainsAny(candidate, `/\`) {
		candidate = "./" + candidate
	}
	rel, err := sandbox.SanitizeArg(base, candidate)
	if err != nil {
		return "", "", err
	}
	if rel == "." || rel == "" {
		return "", "", fmt.Errorf("invalid path")
	}
	full := filepath.Join(base, rel)
	return rel, full, nil
}

func readLimited(path string, maxBytes int) ([]byte, bool, error) {
	if maxBytes <= 0 {
		return nil, false, fmt.Errorf("invalid max_bytes")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	limit := int64(maxBytes) + 1
	lr := &io.LimitedReader{R: f, N: limit}
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, false, err
	}
	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return data, truncated, nil
}

func isText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if !utf8.Valid(data) {
		return false
	}
	return bytes.IndexByte(data, 0) == -1
}

func encodeContent(data []byte) (string, string) {
	if isText(data) {
		return string(data), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(data), "base64"
}

func decodeContent(content, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "utf-8", "utf8", "text", "plain":
		return []byte(content), nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 content")
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported encoding %q", encoding)
	}
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".manifold-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
