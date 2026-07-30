package tools

import "strings"

var friendlyToolTitles = map[string]string{
	"agent_call":               "Delegating a task to a specialist",
	"agent_response":           "Presenting the agent’s answer",
	"apply_patch":              "Applying a targeted code change",
	"ask_agent":                "Asking a specialist for help",
	"code_evolve":              "Searching for a better version of the code",
	"code_qa_judge":            "Reviewing the change for ship-readiness",
	"code_qa_optimize":         "Trying small improvements to the code",
	"code_qa_run":              "Running the code-quality checks",
	"decision_reconstruct":     "Reconstructing why a decision was made",
	"decision_record":          "Recording a decision for future context",
	"decision_review":          "Reviewing a decision’s lifecycle",
	"decision_search":          "Looking up past decisions",
	"delegate_to_team":         "Handing the work to a specialist team",
	"describe_image":           "Taking a closer look at an image",
	"file_delete":              "Deleting a file",
	"file_patch":               "Editing a specific section of a file",
	"file_read":                "Reading project files for context",
	"file_write":               "Writing a file to the workspace",
	"llm_parallel_completions": "Asking several models at once",
	"magma_lifecycle":          "Tending the MAGMA knowledge graph",
	"matrix_room_message":      "Sending a message to the Matrix room",
	"multi_tool_use.parallel":  "Fanning out independent work in parallel",
	"multi_tool_use_parallel":  "Fanning out independent work in parallel",
	"pulse_tasks":              "Managing scheduled tasks",
	"rag_ingest":               "Teaching the knowledge base something new",
	"rag_retrieve":             "Searching memory for relevant knowledge",
	"request_info":             "Pausing to ask for missing information",
	"run_cli":                  "Running a workspace command safely",
	"skill_read":               "Loading a skill’s instructions",
	"skill_search":             "Finding the right skill for the job",
	"split_text":               "Breaking text into useful chunks",
	"terminal_list":            "Checking active terminal sessions",
	"terminal_read":            "Reading live terminal output",
	"terminal_start":           "Opening a live terminal session",
	"terminal_stop":            "Closing a live terminal session",
	"terminal_write":           "Sending input to a live terminal",
	"text_to_speech":           "Turning text into spoken audio",
	"tool_search":              "Finding the right tool for the job",
	"transit_create":           "Sharing a new memory with the project",
	"transit_delete":           "Removing a shared memory",
	"transit_discover":         "Finding shared memories by their metadata",
	"transit_get":              "Opening a shared memory",
	"transit_list_keys":        "Listing shared memory keys",
	"transit_list_recent":      "Catching up on recent shared memories",
	"transit_search":           "Searching shared memory for answers",
	"transit_update":           "Updating a shared memory",
	"utility_textbox":          "Dropping a text box into the workflow",
	"web_fetch":                "Reading a web page",
	"web_screenshot":           "Taking a snapshot of a web page",
	"web_search":               "Searching the web for answers",
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
