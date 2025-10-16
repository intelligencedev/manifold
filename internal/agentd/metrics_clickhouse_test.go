package agentd

import "testing"

func TestSanitizeIdentifier(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		allowDot bool
		ok       bool
	}{
		{"simple", "metrics", false, true},
		{"withDot", "otel.metrics", true, true},
		{"empty", "", false, false},
		{"invalidChar", "metrics-raw", false, false},
		{"leadingDigit", "1metrics", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sanitizeIdentifier(tc.value, tc.allowDot)
			if tc.ok && err != nil {
				t.Fatalf("expected success, got error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error for value %q", tc.value)
			}
		})
	}
}

func TestBuildAttributeExpr(t *testing.T) {
	if _, err := buildAttributeExpr("llm.model"); err != nil {
		t.Fatalf("expected attribute expression to succeed: %v", err)
	}
	if _, err := buildAttributeExpr("llm model"); err == nil {
		t.Fatalf("expected attribute expression to fail for spaces")
	}
}
