package config

import (
	"slices"
	"testing"
)

func TestDefaultAgentToolAllowList(t *testing.T) {
	got := DefaultAgentToolAllowList()
	want := []string{
		"run_cli", "terminal_start", "terminal_read", "terminal_write", "terminal_stop", "terminal_list",
		"web_screenshot", "web_fetch", "file_read", "file_write", "file_patch", "file_delete",
		"multi_tool_use_parallel", "describe_image", "ask_agent", "request_info",
	}
	for _, name := range want {
		if !slices.Contains(got, name) {
			t.Errorf("default allow-list missing %q; got %v", name, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("default allow-list = %v (len %d), want len %d", got, len(got), len(want))
	}
}

func TestDefaultAgentToolAllowListReturnsCopy(t *testing.T) {
	first := DefaultAgentToolAllowList()
	first[0] = "mutated"
	if second := DefaultAgentToolAllowList(); second[0] != "run_cli" {
		t.Fatalf("DefaultAgentToolAllowList returned a shared slice; got %q", second[0])
	}
}
