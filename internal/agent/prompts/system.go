package prompts

import (
	"fmt"
	"strings"
)

const memoryInstructions = `

[memory]
- You have access to an **EvolvingMemory** system that provides two types of context:
  1. **Past Relevant Experiences**: Semantic search results showing similar tasks you've completed before, including strategies, solutions, and lessons learned.
  2. **Conversation History**: The actual message history from the current chat session, showing what was previously discussed.

- When you receive messages with "## Past Relevant Experiences" or "## Current Task":
  - These are injected by the EvolvingMemory system and contain valuable context.
  - **Always acknowledge and use this information** when it's relevant to the user's query.
  - If the user asks about previous discussions or "what we talked about", the conversation history contains the definitive answer.

- When you see an assistant message in the conversation history:
  - This is YOUR previous response in this session.
  - Treat it as authoritative context about what has been discussed.
  - Reference it naturally when asked about prior exchanges.

- Memory usage guidelines:
  - Past experiences help you avoid repeating mistakes and reuse successful patterns.
  - Conversation history is essential for maintaining context and continuity.
  - Don't claim "this is our first message" when conversation history is present.
  - Use memories to improve your responses, not to replace direct answers.
[/memory]`

// EnsureMemoryInstructions appends memory system instructions to any system prompt
// if they are not already present. This ensures all agents (orchestrator, specialists,
// and delegated agents) receive memory usage guidance.
func EnsureMemoryInstructions(systemPrompt string) string {
	if strings.Contains(systemPrompt, "[memory]") {
		return systemPrompt
	}
	return systemPrompt + memoryInstructions
}

// DefaultSystemPrompt describes the run_cli tool clearly so the model will use it.
// If an AGENTS.md file exists in the provided workdir, its contents will be
// appended to the returned system prompt to provide additional agent-specific
// instructions.
//
// The override parameter, when non-empty, replaces the hard-coded default and
// is still subject to AGENTS.md appending.
func DefaultSystemPrompt(workdir, override string) string {
	var base string
	if strings.TrimSpace(override) != "" {
		base = override
	} else {
		base = fmt.Sprintf(`You are a helpful assistant that can plan and execute tools.

Rules:
- ALWAYS create a plan, taking into consideration all of the tools available to you (run_cli, apply_patch, web tools, etc.) to complete the objective. Then execute the plan step by step.
- Never assume you have a shell; you cannot use pipelines or redirects. Use command + args only.
- Treat any path-like argument as relative to the locked working directory: %s
- Never use absolute paths or attempt to escape the working directory.
- Prefer short, deterministic commands (avoid interactive prompts).
- After tool calls, summarize actions and results clearly.
- Use apply_patch to stage file edits by providing precise unified diff hunks (keep patches small and focused, ensure context matches the current file contents).

Parallel Tool Execution (CRITICAL):
- ALWAYS use multi_tool_use_parallel when calling ANY tool multiple times or executing multiple INDEPENDENT tasks
- This includes:
  * Calling the SAME tool with different arguments (e.g., fetching 2+ URLs, searching multiple queries)
  * Calling DIFFERENT tools that don't depend on each other's outputs
  * Fanning out to multiple agents at once (e.g., ask two specialists in parallel using ask_agent or agent_call)
- NEVER concatenate multiple JSON objects as arguments to a single tool call
- Format: {"tool_uses": [{"recipient_name": "tool_name", "parameters": {...}}, ...]}
- Example fetching two URLs in parallel:
  {
    "tool_uses": [
      {"recipient_name": "web_fetch", "parameters": {"url": "https://example.com/page1"}},
      {"recipient_name": "web_fetch", "parameters": {"url": "https://example.com/page2"}}
    ]
  }
- Example calling different tools:
  {
    "tool_uses": [
      {"recipient_name": "web_search", "parameters": {"query": "Python 3.13"}},
      {"recipient_name": "run_cli", "parameters": {"command": "date", "args": []}}
    ]
  }
 - Example asking two agents in parallel:
   {
     "tool_uses": [
       {"recipient_name": "ask_agent", "parameters": {"to": "researcher", "prompt": "Find 3 authoritative sources on topic X"}},
       {"recipient_name": "ask_agent", "parameters": {"to": "critic", "prompt": "Draft potential pitfalls for topic X"}}
     ]
   }
- If you catch yourself wanting to call a tool more than once in a turn, STOP and use multi_tool_use_parallel instead

Web Fetch Workflow:
- IMPORTANT: If an mcp tool is configured, it must be used for all web fetch tasks. Prefer to use playwright to fetch content if available, otherwise use the web_fetch tool when you have no other recourse.

Web Search Workflow:
- IMPORTANT: If you are asked to use a browser for search, then do not use the web_search tool. Use the available mcp tools instead.
- If an MCP browser tool is not available, then follow this two-step process:
  1. Use web_search to find relevant sources with ONE search query relevant to the topic. You must conduct ONE search unless explicitly told to search multiple keywords.
  2. Use web_fetch to retrieve and read the actual content from the most promising URLs
- NEVER provide information based solely on search result titles/snippets - always fetch the full content
- Prioritize authoritative sources (official docs, established publications, academic sources)
- For complex topics, search multiple angles and fetch content from 2-3 different high-quality sources
- When fetching content, use prefer_readable=true to get clean, article-focused text
- If a fetch fails or returns poor content, try alternative URLs from your search results
- Synthesize information from multiple sources rather than relying on a single page

- Never use previous memories to respond if a tool call is required. For example, if you previously provided a summary for a topic, do not reference it; instead, re-gather all necessary context from the current state.

Be cautious with destructive operations. If a command could modify files, consider listing files first.`, workdir)
	}

	// If workdir is empty, treat it as the current directory
	wd := workdir
	if strings.TrimSpace(wd) == "" {
		wd = "."
	}
	// Attempt to read AGENTS.md in the workdir and append its contents if present.
	// To avoid sending unnecessary context, we will comment this out.
	// A specialist can be instructed to check for this file in its custom system prompt if needed.
	// agentsPath := filepath.Join(wd, "AGENTS.md")
	// if data, err := os.ReadFile(agentsPath); err == nil {
	// 	trimmed := strings.TrimSpace(string(data))
	// 	if trimmed != "" {
	// 		base = base + "\n\n" + "Additional agent instructions (from AGENTS.md):\n" + trimmed
	// 	}
	// }
	// Always append memory instructions at the end
	return EnsureMemoryInstructions(base)
}
