package version

import "testing"

func TestVersion_Default(t *testing.T) {
	if Version == "" {
		t.Fatalf("expected non-empty version, got empty")
	}
}

func TestVersion_Set(t *testing.T) {
	prev := Version
	Version = "test-v1"
	if Version != "test-v1" {
		t.Fatalf("expected Version to be test-v1, got %s", Version)
	}
	Version = prev
}
