package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewRegistryBuilds(t *testing.T) {
	_ = NewRegistry()
}

type aliasTestTool struct{}

func (aliasTestTool) Name() string { return "multi_tool_use_parallel" }
func (aliasTestTool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "parallel alias test",
		"parameters":  map[string]any{"type": "object"},
	}
}
func (aliasTestTool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	return map[string]any{"ok": true}, nil
}

func TestRegistryDispatchesDottedParallelAlias(t *testing.T) {
	reg := NewRegistry()
	reg.Register(aliasTestTool{})

	payload, err := reg.Dispatch(context.Background(), "multi_tool_use.parallel", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("dispatch alias: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if parsed["ok"] != true {
		t.Fatalf("expected ok payload, got %s", payload)
	}
}

func TestFilteredRegistryAcceptsDottedParallelAliasInAllowList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(aliasTestTool{})
	filtered := NewFilteredRegistry(reg, []string{"multi_tool_use.parallel"})

	if names := SchemaNames(filtered); len(names) != 1 || names[0] != "multi_tool_use_parallel" {
		t.Fatalf("expected aliased schema to be exposed, got %#v", names)
	}
}

func TestRegistrySchemasPopulateDefaultTitle(t *testing.T) {
	reg := NewRegistry()
	reg.Register(aliasTestTool{})

	schemas := reg.Schemas()
	if len(schemas) != 1 {
		t.Fatalf("schemas = %d, want 1", len(schemas))
	}
	if schemas[0].Title == "" {
		t.Fatalf("expected schema title for %s", schemas[0].Name)
	}
	if got := ToolTitle(reg, "multi_tool_use.parallel"); got == "" {
		t.Fatalf("expected title lookup for dotted alias")
	}
}

func TestHumanizeToolNameUsesFriendlyInternalTitles(t *testing.T) {
	tests := map[string]string{
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

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := HumanizeToolName(name); got != want {
				t.Fatalf("HumanizeToolName(%s) = %q, want %q", name, got, want)
			}
		})
	}
}
