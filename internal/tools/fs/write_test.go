package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTool_Call_WriteAndAppend(t *testing.T) {
	td := t.TempDir()
	w := NewWriteTool(td)
	// write
	args := map[string]any{"path": "subdir/file.txt", "content": "hello", "append": false}
	raw, _ := json.Marshal(args)
	res, err := w.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call returned err: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}
	if okv, _ := m["ok"].(bool); !okv {
		t.Fatalf("expected ok true, got %v", m)
	}
	p := filepath.Join(td, "subdir", "file.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected content: %s", string(b))
	}
	// append
	args = map[string]any{"path": "subdir/file.txt", "content": "-again", "append": true}
	raw, _ = json.Marshal(args)
	res, err = w.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call append err: %v", err)
	}
	b, _ = os.ReadFile(p)
	if string(b) != "hello-again" {
		t.Fatalf("unexpected appended content: %s", string(b))
	}
}

func TestWriteTool_Call_SanitizePath(t *testing.T) {
	td := t.TempDir()
	w := NewWriteTool(td)
	// attempt path traversal
	args := map[string]any{"path": "../etc/passwd", "content": "x", "append": false}
	raw, _ := json.Marshal(args)
	res, err := w.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("Call returned err: %v", err)
	}
	m, _ := res.(map[string]any)
	if okv, _ := m["ok"].(bool); okv {
		t.Fatalf("expected ok false for sanitized path, got true")
	}
}
