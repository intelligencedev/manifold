package mcpclient

import (
	"context"
	"testing"

	"intelligence.dev/internal/config"

	mcppkg "github.com/modelcontextprotocol/go-sdk/mcp"
)

// We'll test that sanitizeName works and that RegisterFromConfig skips bad servers.
func TestSanitizeName(t *testing.T) {
	in := "server:name/with spaces"
	out := sanitizeName(in)
	if out == in {
		t.Fatalf("expected change, got same: %s", out)
	}
	if out == "" {
		t.Fatalf("unexpected empty")
	}
}

func TestRegisterFromConfig_SkipsEmpty(t *testing.T) {
	m := NewManager()
	// Empty config should be fine
	var cfg config.MCPConfig
	if err := m.RegisterFromConfig(context.Background(), nil, cfg); err != nil {
		t.Fatalf("RegisterFromConfig returned error: %v", err)
	}
	_ = m
}

func TestMCPTool_Name(t *testing.T) {
	tool := &mcpTool{server: "s", session: nil, tool: &mcppkg.Tool{Name: "t", Description: "d"}}
	if n := tool.Name(); n == "" {
		t.Fatalf("empty name")
	}
}
