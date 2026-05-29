package filetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxReadBytes  = 64 * 1024
	defaultMaxWriteBytes = 1 * 1024 * 1024
	defaultMaxPatchBytes = 1 * 1024 * 1024
	maxReadBytes         = 4 * 1024 * 1024
)

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
	paths := readArgPaths(args)
	if len(paths) == 0 {
		return readResult{OK: false, Error: "missing path(s)"}, nil
	}

	base, err := t.guard.baseDir(ctx)
	if err != nil {
		return readResult{OK: false, Error: err.Error()}, nil
	}

	maxBytes := t.effectiveMaxBytes(args.MaxBytes)

	results := make([]readFileEntry, 0, len(paths))
	anyOK := false
	for _, p := range paths {
		entry := readOneFile(base, p, maxBytes)
		results = append(results, entry)
		anyOK = anyOK || entry.OK
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

func readArgPaths(args readArgs) []string {
	paths := make([]string, 0, 1+len(args.Paths))
	if args.Path != "" {
		paths = append(paths, args.Path)
	}
	if len(args.Paths) > 0 {
		paths = append(paths, args.Paths...)
	}
	return paths
}

func (t *readTool) effectiveMaxBytes(requested int) int {
	maxBytes := t.defaultMaxBytes
	if requested > 0 {
		maxBytes = requested
	}
	if maxBytes > t.maxBytes {
		maxBytes = t.maxBytes
	}
	if maxBytes <= 0 {
		maxBytes = t.defaultMaxBytes
	}
	return maxBytes
}

func readOneFile(base, path string, maxBytes int) readFileEntry {
	entry := readFileEntry{Path: cleanInputPath(path)}
	rel, full, err := resolvePath(base, path)
	if err != nil {
		entry.Error = fmt.Sprintf("invalid path: %v", err)
		return entry
	}
	info, err := os.Lstat(full)
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		entry.Error = "refusing to read symlink"
		return entry
	}
	if info.IsDir() {
		entry.Error = "path is a directory"
		return entry
	}
	data, truncated, err := readLimited(full, maxBytes)
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	content, encoding := encodeContent(data)
	entry.Path = filepath.ToSlash(rel)
	entry.OK = true
	entry.Content = content
	entry.Encoding = encoding
	entry.Bytes = info.Size()
	entry.BytesRead = len(data)
	entry.Truncated = truncated
	return entry
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

type deleteTool struct {
	guard rootGuard
}

type deleteArgs struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type deleteResult struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Path    string `json:"path,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
	WasDir  bool   `json:"was_dir,omitempty"`
}

func NewDeleteTool(allowedRoots []string) *deleteTool {
	return &deleteTool{guard: newRootGuard(allowedRoots)}
}

func (t *deleteTool) Name() string { return "file_delete" }

func (t *deleteTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Delete a file or directory from the current project workspace. Directories require recursive=true.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":      map[string]any{"type": "string", "description": "File or directory path relative to the project root."},
				"recursive": map[string]any{"type": "boolean", "description": "Allow deleting directories and their contents."},
			},
			"required": []string{"path"},
		},
	}
}

func (t *deleteTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args deleteArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return deleteResult{OK: false, Error: "missing path"}, nil
	}

	base, err := t.guard.baseDir(ctx)
	if err != nil {
		return deleteResult{OK: false, Error: err.Error()}, nil
	}
	rel, full, err := resolvePath(base, args.Path)
	if err != nil {
		return deleteResult{OK: false, Error: fmt.Sprintf("invalid path: %v", err)}, nil
	}
	info, err := os.Lstat(full)
	if err != nil {
		return deleteResult{OK: false, Error: err.Error()}, nil
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return deleteResult{OK: false, Error: "refusing to delete symlink"}, nil
	}

	wasDir := info.IsDir()
	if wasDir && !args.Recursive {
		return deleteResult{OK: false, Error: "path is a directory; set recursive to delete"}, nil
	}
	if wasDir {
		if err := os.RemoveAll(full); err != nil {
			return deleteResult{OK: false, Error: fmt.Sprintf("delete directory: %v", err)}, nil
		}
	} else if err := os.Remove(full); err != nil {
		return deleteResult{OK: false, Error: fmt.Sprintf("delete file: %v", err)}, nil
	}

	return deleteResult{
		OK:      true,
		Path:    filepath.ToSlash(rel),
		Deleted: true,
		WasDir:  wasDir,
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
