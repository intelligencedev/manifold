package fs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPatch_AddAndUpdate(t *testing.T) {
	td := t.TempDir()
	tool := NewApplyPatchTool(td)

	// Start with a file that we will update
	p := filepath.Join(td, "a.txt")
	_ = os.WriteFile(p, []byte("hello world\nline2\n"), 0o644)

	patch := `*** Begin Patch
*** Update File: a.txt
  hello world
- line2
+ line two
*** Add File: b.txt
+first
+second
*** End Patch`
	args := map[string]any{"patch": patch}
	raw, _ := json.Marshal(args)
	res, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("call err: %v", err)
	}
	m, _ := res.(map[string]any)
	if okv, _ := m["ok"].(bool); !okv {
		t.Fatalf("expected ok true, got %v", m)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "hello world\nline two\n" {
		t.Fatalf("unexpected a.txt: %q", string(b))
	}
	b, _ = os.ReadFile(filepath.Join(td, "b.txt"))
	if string(b) != "first\nsecond" {
		t.Fatalf("unexpected b.txt: %q", string(b))
	}
}
