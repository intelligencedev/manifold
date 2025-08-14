package sandbox

import "testing"

func TestSanitizeArgRejectsPathTraversal(t *testing.T) {
	_, err := SanitizeArg("/workdir", "../etc/passwd")
	if err == nil {
		t.Fatalf("expected error for traversal")
	}
}
