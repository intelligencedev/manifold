package config

import (
	"slices"
	"testing"
)

func TestDefaultAgentToolAllowList(t *testing.T) {
	got := DefaultAgentToolAllowList()
	want := []string{
		"run_cli", "web_fetch",
		"transit_create", "transit_get", "transit_update", "transit_delete",
		"transit_search", "transit_discover", "transit_list_keys", "transit_list_recent",
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
