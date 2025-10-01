package tools

import (
	"testing"

	"manifold/internal/tools/patchtool"
)

func TestPatchToolPublishedInRegistry(t *testing.T) {
	reg := NewRegistry()
	reg.Register(patchtool.New("."))
	found := false
	for _, schema := range reg.Schemas() {
		if schema.Name == "apply_patch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected apply_patch tool to be registered")
	}
}
