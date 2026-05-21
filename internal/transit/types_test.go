package transit_test

import (
	"testing"

	transit "manifold/internal/transit"
)

func TestNormalizeKeyName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "trims whitespace", key: " project/demo/brief ", want: "project/demo/brief"},
		{name: "preserves supported characters", key: "Project_1/demo.topic@agent-status", want: "Project_1/demo.topic@agent-status"},
		{name: "replaces spaces and punctuation", key: "Project Demo: Brief!", want: "Project-Demo-Brief-"},
		{name: "collapses repeated replacements", key: "Project :: Demo", want: "Project-Demo"},
		{name: "removes leading and trailing slashes", key: "/project/demo/", want: "project/demo"},
		{name: "collapses double slash", key: "project//demo", want: "project/demo"},
		{name: "collapses double dot", key: "project/demo..brief", want: "project/demo.brief"},
		{name: "keeps nonempty fallback", key: "///", want: "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := transit.NormalizeKeyName(tt.key); got != tt.want {
				t.Fatalf("NormalizeKeyName(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestNormalizeKeyNameProducesValidKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"Project Demo: Brief!",
		"/project//demo../brief/",
		"memory title with spaces",
		"///",
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			normalized := transit.NormalizeKeyName(key)
			if err := transit.ValidateKey(normalized); err != nil {
				t.Fatalf("ValidateKey(%q) after normalization failed: %v", normalized, err)
			}
		})
	}
}
