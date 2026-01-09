package filetool

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

const (
	defaultMaxReadBytes  = 64 * 1024
	defaultMaxWriteBytes = 1 * 1024 * 1024
	defaultMaxPatchBytes = 1 * 1024 * 1024
	maxReadBytes         = 4 * 1024 * 1024
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

type readTool struct {
	guard           rootGuard
	defaultMaxBytes int
	maxBytes        int
}

type readArgs struct {
	Path     string   `json:"path"`
	Paths    []string `json:"paths"`
	MaxBytes int      `json:"max_bytes"`
}

type readFileEntry struct {
	Path      string `json:"path"`
	OK        bool   `json:"ok"`
	Content   string `json:"content,omitempty"`
	Encoding  string `json:"encoding,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`
	BytesRead int    `json:"bytes_read,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Error     string `json:"error,omitempty"`
}

type readResult struct {
	OK        bool            `json:"ok"`
	Error     string          `json:"error,omitempty"`
	Path      string          `json:"path,omitempty"`
	Content   string          `json:"content,omitempty"`
	Encoding  string          `json:"encoding,omitempty"`
	Bytes     int64           `json:"bytes,omitempty"`
	BytesRead int             `json:"bytes_read,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
	Files     []readFileEntry `json:"files,omitempty"`
}

func NewReadTool(allowedRoots []string, defaultMaxBytes int) *readTool {
	if defaultMaxBytes <= 0 {
		defaultMaxBytes = defaultMaxReadBytes
	}
	if defaultMaxBytes > maxReadBytes {
		defaultMaxBytes = maxReadBytes
	}
	return &readTool{
		guard:           newRootGuard(allowedRoots),
		defaultMaxBytes: defaultMaxBytes,
		maxBytes:        maxReadBytes,
	}
}

func (t *readTool) Name() string { return "file_read" }

func (t *readTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Read one or more files from the current project workspace. Paths are relative to the project root.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":      map[string]any{"type": "string", "description": "Single file path relative to the project root."},
				"paths":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Multiple file paths relative to the project root."},
				"max_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": maxReadBytes, "description": "Maximum bytes to read per file (defaults to output truncation limit)."},
			},
		},
	}
}

func (t *readTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	paths := make([]string, 0, 1+len(args.Paths))
	if args.Path != "" {
		paths = append(paths, args.Path)
	}
	if len(args.Paths) > 0 {
		paths = append(paths, args.Paths...)
	}
	if len(paths) == 0 {
		return readResult{OK: false, Error: "missing path(s)"}, nil
	}

	base, err := t.guard.baseDir(ctx)
	if err != nil {
		return readResult{OK: false, Error: err.Error()}, nil
	}

	maxBytes := t.defaultMaxBytes
	if args.MaxBytes > 0 {
		maxBytes = args.MaxBytes
	}
	if maxBytes > t.maxBytes {
		maxBytes = t.maxBytes
	}
	if maxBytes <= 0 {
		maxBytes = t.defaultMaxBytes
	}

	results := make([]readFileEntry, 0, len(paths))
	anyOK := false
	for _, p := range paths {
		entry := readFileEntry{Path: cleanInputPath(p)}
		rel, full, err := resolvePath(base, p)
		if err != nil {
			entry.Error = fmt.Sprintf("invalid path: %v", err)
			results = append(results, entry)
			continue
		}
		info, err := os.Lstat(full)
		if err != nil {
			entry.Error = err.Error()
			results = append(results, entry)
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			entry.Error = "refusing to read symlink"
			results = append(results, entry)
			continue
		}
		if info.IsDir() {
			entry.Error = "path is a directory"
			results = append(results, entry)
			continue
		}
		data, truncated, err := readLimited(full, maxBytes)
		if err != nil {
			entry.Error = err.Error()
			results = append(results, entry)
			continue
		}
		content, encoding := encodeContent(data)
		entry.Path = filepath.ToSlash(rel)
		entry.OK = true
		entry.Content = content
		entry.Encoding = encoding
		entry.Bytes = info.Size()
		entry.BytesRead = len(data)
		entry.Truncated = truncated
		results = append(results, entry)
		anyOK = true
	}

	if len(paths) == 1 && args.Path != "" && len(args.Paths) == 0 {
		entry := results[0]
		return readResult{
			OK:        entry.OK,
			Error:     entry.Error,
			Path:      entry.Path,
			Content:   entry.Content,
			Encoding:  entry.Encoding,
			Bytes:     entry.Bytes,
			BytesRead: entry.BytesRead,
			Truncated: entry.Truncated,
		}, nil
	}

	if !anyOK {
		return readResult{OK: false, Error: "no files could be read", Files: results}, nil
	}
	return readResult{OK: true, Files: results}, nil
}

type writeTool struct {
	guard    rootGuard
	maxBytes int
}

type writeArgs struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type writeResult struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Path    string `json:"path,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	Created bool   `json:"created,omitempty"`
}

func NewWriteTool(allowedRoots []string, maxBytes int) *writeTool {
	if maxBytes <= 0 {
		maxBytes = defaultMaxWriteBytes
	}
	return &writeTool{
		guard:    newRootGuard(allowedRoots),
		maxBytes: maxBytes,
	}
}

func (t *writeTool) Name() string { return "file_write" }

func (t *writeTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Create or overwrite a single file in the current project workspace.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":     map[string]any{"type": "string", "description": "File path relative to the project root."},
				"content":  map[string]any{"type": "string", "description": "File contents."},
				"encoding": map[string]any{"type": "string", "description": "Content encoding: utf-8 (default) or base64."},
			},
			"required": []string{"path", "content"},
		},
	}
}

func (t *writeTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args writeArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return writeResult{OK: false, Error: "missing path"}, nil
	}

	base, err := t.guard.baseDir(ctx)
	if err != nil {
		return writeResult{OK: false, Error: err.Error()}, nil
	}
	rel, full, err := resolvePath(base, args.Path)
	if err != nil {
		return writeResult{OK: false, Error: fmt.Sprintf("invalid path: %v", err)}, nil
	}
	data, err := decodeContent(args.Content, args.Encoding)
	if err != nil {
		return writeResult{OK: false, Error: err.Error()}, nil
	}
	if t.maxBytes > 0 && len(data) > t.maxBytes {
		return writeResult{OK: false, Error: fmt.Sprintf("content exceeds limit (%d > %d bytes)", len(data), t.maxBytes)}, nil
	}

	var perm fs.FileMode = 0o644
	created := true
	if info, err := os.Lstat(full); err == nil {
		created = false
		if info.Mode()&fs.ModeSymlink != 0 {
			return writeResult{OK: false, Error: "refusing to overwrite symlink"}, nil
		}
		if info.IsDir() {
			return writeResult{OK: false, Error: "path is a directory"}, nil
		}
		perm = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return writeResult{OK: false, Error: err.Error()}, nil
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return writeResult{OK: false, Error: fmt.Sprintf("create directories: %v", err)}, nil
	}
	if err := writeFileAtomic(full, data, perm); err != nil {
		return writeResult{OK: false, Error: fmt.Sprintf("write file: %v", err)}, nil
	}

	return writeResult{
		OK:      true,
		Path:    filepath.ToSlash(rel),
		Bytes:   len(data),
		Created: created,
	}, nil
}

type patchTool struct {
	guard        rootGuard
	maxFileBytes int64
}

type patchArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
}

type patchResult struct {
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	Path         string `json:"path,omitempty"`
	StartLine    int    `json:"start_line,omitempty"`
	EndLine      int    `json:"end_line,omitempty"`
	LineCount    int    `json:"line_count,omitempty"`
	NewLineCount int    `json:"new_line_count,omitempty"`
}

func NewPatchTool(allowedRoots []string, maxFileBytes int64) *patchTool {
	if maxFileBytes <= 0 {
		maxFileBytes = defaultMaxPatchBytes
	}
	return &patchTool{
		guard:        newRootGuard(allowedRoots),
		maxFileBytes: maxFileBytes,
	}
}

func (t *patchTool) Name() string { return "file_patch" }

func (t *patchTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Replace a specific line range in a single file without modifying other lines.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string", "description": "File path relative to the project root."},
				"start_line": map[string]any{"type": "integer", "minimum": 1, "description": "1-based start line (inclusive)."},
				"end_line":   map[string]any{"type": "integer", "minimum": 1, "description": "1-based end line (inclusive)."},
				"content":    map[string]any{"type": "string", "description": "Replacement text for the specified line range."},
			},
			"required": []string{"path", "start_line", "end_line", "content"},
		},
	}
}

func (t *patchTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args patchArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return patchResult{OK: false, Error: "missing path"}, nil
	}
	if args.StartLine <= 0 || args.EndLine <= 0 || args.EndLine < args.StartLine {
		return patchResult{OK: false, Error: "invalid line range"}, nil
	}

	base, err := t.guard.baseDir(ctx)
	if err != nil {
		return patchResult{OK: false, Error: err.Error()}, nil
	}
	rel, full, err := resolvePath(base, args.Path)
	if err != nil {
		return patchResult{OK: false, Error: fmt.Sprintf("invalid path: %v", err)}, nil
	}
	info, err := os.Lstat(full)
	if err != nil {
		return patchResult{OK: false, Error: err.Error()}, nil
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return patchResult{OK: false, Error: "refusing to patch symlink"}, nil
	}
	if info.IsDir() {
		return patchResult{OK: false, Error: "path is a directory"}, nil
	}
	if t.maxFileBytes > 0 && info.Size() > t.maxFileBytes {
		return patchResult{OK: false, Error: fmt.Sprintf("file exceeds size limit (%d > %d bytes)", info.Size(), t.maxFileBytes)}, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return patchResult{OK: false, Error: err.Error()}, nil
	}
	if !isText(data) {
		return patchResult{OK: false, Error: "file is not valid UTF-8 text"}, nil
	}

	lines, trailing, newline := splitLines(data)
	if len(lines) == 0 {
		return patchResult{OK: false, Error: "file is empty"}, nil
	}
	lineCount := len(lines)
	if args.StartLine > lineCount || args.EndLine > lineCount {
		return patchResult{OK: false, Error: fmt.Sprintf("line range exceeds file length (%d)", lineCount)}, nil
	}

	replacement := splitReplacementLines(args.Content, newline)
	startIdx := args.StartLine - 1
	endIdx := args.EndLine - 1

	updated := make([]string, 0, lineCount-(endIdx-startIdx+1)+len(replacement))
	updated = append(updated, lines[:startIdx]...)
	updated = append(updated, replacement...)
	if endIdx+1 < len(lines) {
		updated = append(updated, lines[endIdx+1:]...)
	}

	out := strings.Join(updated, newline)
	if trailing && len(updated) > 0 {
		out += newline
	}

	if err := writeFileAtomic(full, []byte(out), info.Mode().Perm()); err != nil {
		return patchResult{OK: false, Error: fmt.Sprintf("write file: %v", err)}, nil
	}

	return patchResult{
		OK:           true,
		Path:         filepath.ToSlash(rel),
		StartLine:    args.StartLine,
		EndLine:      args.EndLine,
		LineCount:    lineCount,
		NewLineCount: len(updated),
	}, nil
}

func splitLines(data []byte) ([]string, bool, string) {
	rawContent := string(data)
	newline := "\n"
	if strings.Contains(rawContent, "\r\n") {
		newline = "\r\n"
	}
	trailing := len(data) > 0 && data[len(data)-1] == '\n'
	raw := strings.Split(rawContent, "\n")
	if trailing && len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	if newline == "\r\n" {
		for i, line := range raw {
			raw[i] = strings.TrimSuffix(line, "\r")
		}
	}
	return raw, trailing, newline
}

func splitReplacementLines(content, newline string) []string {
	if content == "" {
		return nil
	}
	raw := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") && len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	if newline == "\r\n" {
		for i, line := range raw {
			raw[i] = strings.TrimSuffix(line, "\r")
		}
	}
	return raw
}
