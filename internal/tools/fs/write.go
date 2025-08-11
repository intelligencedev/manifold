package fs

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"

    "gptagent/internal/sandbox"
)

// WriteTool writes text content to a file within the locked WORKDIR.
type WriteTool struct{ workdir string }

func NewWriteTool(workdir string) *WriteTool { return &WriteTool{workdir: workdir} }

func (t *WriteTool) Name() string { return "write_file" }

func (t *WriteTool) JSONSchema() map[string]any {
    return map[string]any{
        "name":        t.Name(),
        "description": "Write text content to a file in the locked working directory (creates directories as needed).",
        "parameters": map[string]any{
            "type": "object",
            "properties": map[string]any{
                "path":    map[string]any{"type": "string", "description": "Relative path under WORKDIR to write (e.g., report.md)"},
                "content": map[string]any{"type": "string", "description": "Text content to write"},
                "append":  map[string]any{"type": "boolean", "description": "Append to the file instead of overwriting", "default": false},
            },
            "required": []string{"path", "content"},
        },
    }
}

func (t *WriteTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
    var args struct {
        Path    string `json:"path"`
        Content string `json:"content"`
        Append  bool   `json:"append"`
    }
    if err := json.Unmarshal(raw, &args); err != nil { return nil, err }
    rel, err := sandbox.SanitizeArg(t.workdir, args.Path)
    if err != nil { return map[string]any{"ok": false, "error": err.Error()}, nil }
    full := filepath.Join(t.workdir, rel)
    if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    flag := os.O_CREATE | os.O_WRONLY
    if args.Append { flag |= os.O_APPEND } else { flag |= os.O_TRUNC }
    f, err := os.OpenFile(full, flag, 0o644)
    if err != nil { return map[string]any{"ok": false, "error": err.Error()}, nil }
    defer f.Close()
    if _, err := f.WriteString(args.Content); err != nil {
        return map[string]any{"ok": false, "error": err.Error()}, nil
    }
    return map[string]any{"ok": true, "path": rel, "bytes": len(args.Content)}, nil
}
