package agentd

import "testing"

func TestSanitizeGeneratedTitle(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "strips quotes and whitespace", input: "\"  Launch Plan\nOverview  \"", expected: "Launch Plan Overview"},
		{name: "truncates long titles", input: "This is a very long proposed title that should be trimmed at some point", expected: truncateRunes("This is a very long proposed title that should be trimmed at some point", chatTitleMaxRunes)},
		{name: "removes punctuation", input: "-Deep Dive!?", expected: "Deep Dive"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeGeneratedTitle(tc.input)
			if got != tc.expected {
				t.Fatalf("sanitizeGeneratedTitle(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestFallbackChatTitle(t *testing.T) {
	testCases := []struct {
		name     string
		prompt   string
		expected string
	}{
		{name: "blank", prompt: "", expected: "Conversation"},
		{name: "collapses whitespace", prompt: "Find   logs   for   job", expected: "Find logs for job"},
		{name: "truncates", prompt: "A long description that should be shortened by the fallback title helper to avoid overflow", expected: truncateRunes("A long description that should be shortened by the fallback title helper to avoid overflow", chatTitleMaxRunes)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fallbackChatTitle(tc.prompt); got != tc.expected {
				t.Fatalf("fallbackChatTitle(%q) = %q, want %q", tc.prompt, got, tc.expected)
			}
		})
	}
}

func TestIsDefaultSessionName(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want bool
	}{
		{name: "new chat", val: "New Chat", want: true},
		{name: "conversation", val: " conversation ", want: true},
		{name: "empty", val: " ", want: true},
		{name: "custom", val: "Incident review", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDefaultSessionName(tc.val); got != tc.want {
				t.Fatalf("isDefaultSessionName(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}
