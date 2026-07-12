package toolnode

import (
	"fmt"
	"strings"

	"manifold/internal/warpp"
)

func in(name, typ string, required bool, def any, desc string) warpp.PortSpec {
	return warpp.PortSpec{Name: name, Type: typ, Required: required, Default: def, Description: desc}
}

func out(name, typ string) warpp.PortSpec {
	return warpp.PortSpec{Name: name, Type: typ}
}

// rawOut is the whole-result JSON output every adapter exposes.
var rawOut = out("raw", "json")

// Builtin returns the curated tool adapters (spec §6). Each node's title is
// forced to the actual registry tool name so Flow nodes show the real tool,
// not a friendly label.
func Builtin() []Adapter {
	adapters := builtinAdapters()
	for i := range adapters {
		adapters[i].Manifest.Title = adapters[i].Tool
	}
	return adapters
}

func builtinAdapters() []Adapter {
	return []Adapter{
		{
			NodeType: "tool.web_search", Tool: "web_search",
			Manifest: warpp.Manifest{
				Type: "tool.web_search", Title: "Web Search", Category: "tool",
				Description: "Search the web and return ranked results.",
				Inputs: []warpp.PortSpec{
					in("query", "text", true, nil, "Search query."),
					in("max_results", "number", false, float64(5), "Maximum results."),
				},
				Outputs: []warpp.PortSpec{out("results", "list<json>"), out("results_text", "text"), rawOut},
			},
			Args: []ArgMap{{Port: "query"}, {Port: "max_results"}},
			Outs: []OutMap{{Port: "results", Path: "results"}, {Port: "results_text", Path: "results_text"}, {Port: "raw"}},
			Post: webSearchPost,
		},
		{
			NodeType: "tool.web_fetch", Tool: "web_fetch",
			Manifest: warpp.Manifest{
				Type: "tool.web_fetch", Title: "Web Fetch", Category: "tool",
				Description: "Fetch a URL and return Markdown.",
				Inputs:      []warpp.PortSpec{in("url", "text", true, nil, "Absolute URL.")},
				Outputs:     []warpp.PortSpec{out("markdown", "text"), out("url", "text"), rawOut},
			},
			Args: []ArgMap{{Port: "url"}},
			Outs: []OutMap{{Port: "markdown", Path: "markdown"}, {Port: "url", Path: "final_url"}, {Port: "raw"}},
		},
		{
			NodeType: "tool.file_read", Tool: "file_read",
			Manifest: warpp.Manifest{
				Type: "tool.file_read", Title: "Read File", Category: "tool",
				Description: "Read a file from the project workspace.",
				Inputs: []warpp.PortSpec{
					in("path", "file", true, nil, "Path relative to the project root."),
					in("max_bytes", "number", false, nil, "Maximum bytes to read."),
				},
				Outputs: []warpp.PortSpec{out("content", "text"), rawOut},
			},
			Args: []ArgMap{{Port: "path"}, {Port: "max_bytes"}},
			Outs: []OutMap{{Port: "content", Path: "files.0.content"}, {Port: "raw"}},
		},
		{
			NodeType: "tool.file_write", Tool: "file_write",
			Manifest: warpp.Manifest{
				Type: "tool.file_write", Title: "Write File", Category: "tool",
				Description: "Write a file to the project workspace.",
				Inputs: []warpp.PortSpec{
					in("path", "file", true, nil, "Path relative to the project root."),
					in("content", "text", true, nil, "File contents."),
					in("encoding", "text", false, nil, "utf-8 (default) or base64."),
				},
				Outputs: []warpp.PortSpec{out("path", "file"), rawOut},
			},
			Args: []ArgMap{{Port: "path"}, {Port: "content"}, {Port: "encoding"}},
			Outs: []OutMap{{Port: "path", Path: "path"}, {Port: "raw"}},
		},
		{
			NodeType: "tool.run_cli", Tool: "run_cli",
			Manifest: warpp.Manifest{
				Type: "tool.run_cli", Title: "Run CLI", Category: "tool",
				Description: "Run a workspace command.",
				Inputs: []warpp.PortSpec{
					in("command", "text", true, nil, "Bare binary name."),
					in("args", "list<text>", false, nil, "Command arguments."),
					in("stdin", "text", false, nil, "Standard input."),
					in("timeout_seconds", "number", false, nil, "Timeout in seconds."),
				},
				Outputs: []warpp.PortSpec{out("stdout", "text"), out("exit_code", "number"), rawOut},
			},
			Args: []ArgMap{{Port: "command"}, {Port: "args"}, {Port: "stdin"}, {Port: "timeout_seconds"}},
			Outs: []OutMap{{Port: "stdout", Path: "stdout"}, {Port: "exit_code", Path: "exit_code"}, {Port: "raw"}},
		},
		{
			NodeType: "tool.rag_retrieve", Tool: "rag_retrieve",
			Manifest: warpp.Manifest{
				Type: "tool.rag_retrieve", Title: "RAG Retrieve", Category: "tool",
				Description: "Hybrid retrieval over the knowledge base.",
				Inputs: []warpp.PortSpec{
					in("query", "text", true, nil, "Retrieval query."),
					in("k", "number", false, nil, "Number of results."),
					in("include_text", "boolean", false, nil, "Include chunk text."),
				},
				Outputs: []warpp.PortSpec{out("results", "list<json>"), rawOut},
			},
			Args: []ArgMap{{Port: "query"}, {Port: "k"}, {Port: "include_text"}},
			Outs: []OutMap{{Port: "results", Path: "items"}, {Port: "raw"}},
		},
		{
			NodeType: "tool.rag_ingest", Tool: "rag_ingest",
			Manifest: warpp.Manifest{
				Type: "tool.rag_ingest", Title: "RAG Ingest", Category: "tool",
				Description: "Ingest a document into the knowledge base.",
				Inputs: []warpp.PortSpec{
					in("id", "text", true, nil, "Document ID."),
					in("text", "text", true, nil, "Document text."),
					in("title", "text", false, nil, "Title."),
					in("url", "text", false, nil, "Source URL."),
				},
				Outputs: []warpp.PortSpec{rawOut},
			},
			Args: []ArgMap{{Port: "id"}, {Port: "text"}, {Port: "title"}, {Port: "url"}},
			Outs: []OutMap{{Port: "raw"}},
		},
		{
			NodeType: "tool.agent_call", Tool: "agent_call",
			Manifest: warpp.Manifest{
				Type: "tool.agent_call", Title: "Agent Call", Category: "tool",
				Description: "Delegate a prompt to a specialist agent.",
				Inputs:      []warpp.PortSpec{in("prompt", "text", true, nil, "Prompt for the agent.")},
				Outputs:     []warpp.PortSpec{out("text", "text"), rawOut},
			},
			Args: []ArgMap{{Port: "prompt"}},
			Outs: []OutMap{{Port: "text", Path: "output"}, {Port: "raw"}},
		},
		{
			NodeType: "tool.matrix_room_message", Tool: "matrix_room_message",
			Manifest: warpp.Manifest{
				Type: "tool.matrix_room_message", Title: "Matrix Message", Category: "tool",
				Description: "Send a message to the configured Matrix room.",
				Inputs:      []warpp.PortSpec{in("text", "text", true, nil, "Message body.")},
				Outputs:     []warpp.PortSpec{rawOut},
			},
			Args: []ArgMap{{Port: "text"}},
			Outs: []OutMap{{Port: "raw"}},
		},
	}
}

// webSearchPost adds a human-readable results_text field to the search result.
func webSearchPost(result map[string]any) map[string]any {
	items, _ := result["results"].([]any)
	var b strings.Builder
	for i, it := range items {
		m, _ := it.(map[string]any)
		title, _ := m["title"].(string)
		url, _ := m["url"].(string)
		fmt.Fprintf(&b, "%d. %s — %s\n", i+1, title, url)
	}
	result["results_text"] = strings.TrimRight(b.String(), "\n")
	return result
}
