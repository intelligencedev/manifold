package config

// defaultAgentToolAllowList is the bootstrap tool allow-list applied to newly
// created orchestrators and specialists. Keep this list focused on the core
// tools needed for a useful first run; explicit configuration can override it.
var defaultAgentToolAllowList = []string{
	"run_cli",
	"terminal_start",
	"terminal_read",
	"terminal_write",
	"terminal_stop",
	"terminal_list",
	"web_screenshot",
	"web_fetch",
	"file_read",
	"file_write",
	"file_patch",
	"file_delete",
	"multi_tool_use_parallel",
	"describe_image",
	"ask_agent",
	"request_info",
}

// DefaultAgentToolAllowList returns a copy of the default bootstrap allow-list
// for newly created agents (orchestrators and specialists).
func DefaultAgentToolAllowList() []string {
	return append([]string(nil), defaultAgentToolAllowList...)
}
