package warpp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveWorkflowRoundTrip(t *testing.T) {
	dir := t.TempDir()
	w := Workflow{
		Intent:      "TestIntent",
		Description: "desc",
		Keywords:    []string{"a"},
		Steps: []Step{
			{ID: "s1", Text: "do"},
		},
		UI: &WorkflowUI{
			Layout: map[string]NodeLayout{
				"s1": {X: 42, Y: 99},
			},
		},
	}
	path, err := SaveWorkflow(dir, w)
	if err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	r, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	got, err := r.Get("TestIntent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "desc" {
		t.Fatalf("unexpected description: %s", got.Description)
	}
	if got.UI == nil || got.UI.Layout["s1"].X != 42 {
		t.Fatalf("expected layout metadata round-trip")
	}
	if r.Path("TestIntent") != path {
		t.Fatalf("expected path %s, got %s", path, r.Path("TestIntent"))
	}
}

func TestSaveWorkflowToPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	w := Workflow{Intent: "flow", Steps: []Step{{ID: "s1", Text: "x"}}}
	if err := SaveWorkflowToPath(path, w); err != nil {
		t.Fatalf("SaveWorkflowToPath: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("expected bytes written")
	}
}

func TestRegistryUpsertAndRemove(t *testing.T) {
	r := &Registry{}
	w := Workflow{Intent: "flow", Steps: []Step{{ID: "s1", Text: "x"}}}
	r.Upsert(w, "/tmp/flow.json")
	if _, err := r.Get("flow"); err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if got := r.Path("flow"); got != "/tmp/flow.json" {
		t.Fatalf("unexpected path: %s", got)
	}
	r.Remove("flow")
	if _, err := r.Get("flow"); err == nil {
		t.Fatalf("expected missing workflow after remove")
	}
}
