package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configpkg "manifold/internal/config"
)

func TestParseReAct(t *testing.T) {
	input := "Thought: thinking\nAction: code_eval\nAction Input: {\"foo\":1}\nObservation: done"
	th, act, in := parseReAct(input)
	if th != "thinking" || act != "code_eval" || in != "{\"foo\":1}" {
		t.Errorf("unexpected parse: %q %q %q", th, act, in)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Errorf("truncate returned %q", got)
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate returned %q", got)
	}
}

func TestNormalizeMCPArg(t *testing.T) {
	tmp := t.TempDir()
	ae := &AgentEngine{Config: &configpkg.Config{DataPath: tmp}}
	in := `{"host_path":"/mnt/tmp/foo.txt"}`
	out, err := ae.normalizeMCPArg(in)
	if err != nil {
		t.Fatalf("normalizeMCPArg error: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	expect := filepath.Join(tmp, "tmp", "foo.txt")
	if m["host_path"] != expect || m["path"] != expect {
		t.Errorf("paths not normalized: %+v", m)
	}
}

func TestStagePath(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	ae := &AgentEngine{Config: &configpkg.Config{DataPath: tmp}}
	arg := fmt.Sprintf(`{"src":"%s","dest":"dst.txt"}`, src)
	out, err := ae.stagePath(arg)
	if err != nil {
		t.Fatalf("stagePath: %v", err)
	}
	var resp struct {
		HostPath string `json:"host_path"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	copied := filepath.Join(tmp, "tmp", "dst.txt")
	if resp.HostPath != copied {
		t.Errorf("expected host_path %s, got %s", copied, resp.HostPath)
	}
	if _, err := os.Stat(copied); err != nil {
		t.Errorf("copied file missing: %v", err)
	}
}

func TestExecToolOrchestratorBlocksDirectCalls(t *testing.T) {
	ae := &AgentEngine{isolatedToServer: ""} // orchestrator

	// Test that MCP tools are blocked
	mcpTests := []string{
		"some_mcp_tool::action",
		"manifold::cli",
		"postgres::query",
	}

	for _, toolName := range mcpTests {
		t.Run(toolName, func(t *testing.T) {
			_, err := ae.execTool(context.Background(), nil, toolName, "{}", nil)
			if err == nil || !strings.Contains(err.Error(), "orchestrator cannot call MCP tool") {
				t.Fatalf("expected orchestrator to block MCP tool call to %s, got: %v", toolName, err)
			}
		})
	}

	// Test that unknown tools are blocked
	unknownTests := []string{
		"random_tool",
		"unknown_action",
	}

	for _, toolName := range unknownTests {
		t.Run(toolName, func(t *testing.T) {
			_, err := ae.execTool(context.Background(), nil, toolName, "{}", nil)
			if err == nil || !strings.Contains(err.Error(), "orchestrator cannot call") {
				t.Fatalf("expected orchestrator to block unknown tool call to %s, got: %v", toolName, err)
			}
		})
	}
}

func TestExecToolOrchestratorAllowsGenericTools(t *testing.T) {
	ae := &AgentEngine{isolatedToServer: ""} // orchestrator

	// Test finish is allowed
	result, err := ae.execTool(context.Background(), nil, "finish", "test result", nil)
	if err != nil {
		t.Fatalf("expected finish to be allowed, got error: %v", err)
	}
	if result != "test result" {
		t.Errorf("expected 'test result', got '%s'", result)
	}

	// Test generic tools are allowed (these will fail during execution but should pass the gate)
	genericTools := []string{
		"code_eval",
		"web_search",
		"web_fetch",
		"stage_path",
	}

	for _, toolName := range genericTools {
		t.Run(toolName, func(t *testing.T) {
			// These will fail in execution but should pass the orchestrator gate
			_, err := ae.execTool(context.Background(), nil, toolName, "{}", nil)
			// Should not get the "orchestrator cannot call" error
			if err != nil && strings.Contains(err.Error(), "orchestrator cannot call") {
				t.Fatalf("expected generic tool %s to be allowed through orchestrator gate, got: %v", toolName, err)
			}
		})
	}
}
