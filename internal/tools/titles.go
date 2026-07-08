package tools

import "strings"

var friendlyToolTitles = map[string]string{
	"agent_call":              "Call Agent",
	"agent_response":          "Respond",
	"apply_patch":             "Apply Patch",
	"ask_agent":               "Ask Specialist",
	"code_evolve":             "Evolve Code",
	"code_qa_judge":           "Judge Code Quality",
	"code_qa_optimize":        "Optimize Code",
	"code_qa_run":             "Run Code QA",
	"decision_reconstruct":    "Reconstruct Decision",
	"decision_record":         "Record Decision",
	"decision_review":         "Review Decision",
	"decision_search":         "Search Decisions",
	"delegate_to_team":        "Delegate to Team",
	"describe_image":          "Describe Image",
	"file_delete":             "Delete File",
	"file_patch":              "Patch File",
	"file_read":               "Read File",
	"file_write":              "Write File",
	"llm_parallel":            "Run Parallel LLMs",
	"magma_lifecycle":         "Manage MAGMA Graph",
	"matrix_send_message":     "Send Matrix Message",
	"multi_tool_use.parallel": "Use Tools in Parallel",
	"multi_tool_use_parallel": "Use Tools in Parallel",
	"pulse_tasks":             "Manage Pulse Tasks",
	"rag_ingest":              "Ingest RAG Memory",
	"rag_retrieve":            "Retrieve RAG Memory",
	"request_info":            "Request Information",
	"run_cli":                 "Run Command",
	"skill_read":              "Read Skill",
	"skill_search":            "Search Skills",
	"split_text":              "Split Text",
	"terminal_list":           "List Terminals",
	"terminal_read":           "Read Terminal",
	"terminal_start":          "Start Terminal",
	"terminal_stop":           "Stop Terminal",
	"terminal_write":          "Write Terminal",
	"text_to_speech":          "Generate Speech",
	"tool_search":             "Search Tools",
	"transit_create":          "Create Transit Record",
	"transit_delete":          "Delete Transit Record",
	"transit_discover":        "Discover Transit Records",
	"transit_get":             "Get Transit Record",
	"transit_list_keys":       "List Transit Keys",
	"transit_list_recent":     "List Recent Transit Records",
	"transit_search":          "Search Transit Records",
	"transit_update":          "Update Transit Record",
	"utility_textbox":         "Render Text Box",
	"web_fetch":               "Fetch Web Page",
	"web_screenshot":          "Capture Web Screenshot",
	"web_search":              "Search Web",
}

// EnsureToolSchemaTitle returns a copy of schema with a title populated when missing.
func EnsureToolSchemaTitle(schema map[string]any, name string) map[string]any {
	if schema == nil {
		schema = map[string]any{}
	}
	if strings.TrimSpace(strFrom(schema["title"])) != "" {
		return schema
	}
	schema["title"] = HumanizeToolName(name)
	return schema
}

// HumanizeToolName converts a canonical tool name into a concise display title.
func HumanizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Tool"
	}
	if title, ok := friendlyToolTitles[name]; ok {
		return title
	}
	name = strings.TrimPrefix(name, "functions.")
	if title, ok := friendlyToolTitles[name]; ok {
		return title
	}
	if strings.HasPrefix(name, "warpp_") {
		return "Run Workflow"
	}
	name = strings.ReplaceAll(name, "multi_tool_use.parallel", "multi_tool_use_parallel")
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		upper := strings.ToUpper(part)
		switch upper {
		case "CLI", "MCP", "RAG", "MAGMA", "QA", "TTS", "HTTP", "URL", "JSON", "LLM", "PTY":
			out = append(out, upper)
		default:
			out = append(out, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
		}
	}
	if len(out) == 0 {
		return "Tool"
	}
	return strings.Join(out, " ")
}
