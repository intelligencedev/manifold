package config

// defaultAgentToolAllowList is the bootstrap tool allow-list applied to newly
// created orchestrators and specialists. It is intentionally lean — auto
// discovery expands it at runtime. Keep in sync with the tool Name()s in
// internal/tools (run_cli, web_fetch, and the transit_* tools).
var defaultAgentToolAllowList = []string{
	"run_cli",
	"web_fetch",
	"transit_create",
	"transit_get",
	"transit_update",
	"transit_delete",
	"transit_search",
	"transit_discover",
	"transit_list_keys",
	"transit_list_recent",
}

// DefaultAgentToolAllowList returns a copy of the default bootstrap allow-list
// for newly created agents (orchestrators and specialists).
func DefaultAgentToolAllowList() []string {
	return append([]string(nil), defaultAgentToolAllowList...)
}
