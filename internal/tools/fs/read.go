package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"singularityio/internal/sandbox"
)

// ReadTool reads text content from a file within the locked WORKDIR.
type ReadTool struct{ workdir string }

func NewReadTool(workdir string) *ReadTool { return &ReadTool{workdir: workdir} }

func (t *ReadTool) Name() string { return "read_file" }

func (t *ReadTool) JSONSchema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": "Read text content from a file in the locked working directory.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Relative path under WORKDIR (e.g., main.go)"},
			},
			"required": []string{"path"},
		},
	}
}

func (t *ReadTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, err
	}
	rel, err := sandbox.SanitizeArg(t.workdir, args.Path)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	full := filepath.Join(t.workdir, rel)
	b, err := os.ReadFile(full)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "path": rel, "content": string(b)}, nil
}
