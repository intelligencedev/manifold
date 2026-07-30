package observability

import "testing"

func TestConsoleLoggingRequested(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "yes", "on", "  1  "}
	for _, v := range truthy {
		t.Run("truthy/"+v, func(t *testing.T) {
			t.Setenv("MANIFOLD_LOG_STDOUT", v)
			if !ConsoleLoggingRequested() {
				t.Fatalf("expected ConsoleLoggingRequested()=true for %q", v)
			}
		})
	}

	falsy := []string{"", "0", "false", "FALSE", "no", "off"}
	for _, v := range falsy {
		t.Run("falsy/"+v, func(t *testing.T) {
			t.Setenv("MANIFOLD_LOG_STDOUT", v)
			if ConsoleLoggingRequested() {
				t.Fatalf("expected ConsoleLoggingRequested()=false for %q", v)
			}
		})
	}
}
