# Available Tools

This document lists the tools available to the assistant (namespace: `functions`) and summarizes their purpose, input parameters, constraints, and usage examples. Use this as a quick reference when building workflows that call these tools.

---

## Table of Contents

- run_cli
- web_search
- web_fetch
- search_index
- search_query
- search_remove
- vector_upsert
- vector_query
- vector_delete
- graph_upsert_node
- graph_upsert_edge
- graph_neighbors
- graph_get_node
- llm_transform
- describe_image
- specialists_infer
- mcp_filesystem_copy_file
- mcp_filesystem_create_directory
- mcp_filesystem_get_file_info
- mcp_filesystem_list_allowed_directories
- mcp_filesystem_list_directory
- mcp_filesystem_move_file
- mcp_filesystem_read_file
- mcp_filesystem_read_multiple_files
- mcp_filesystem_search_files
- mcp_filesystem_tree
- mcp_filesystem_write_file
- multi_tool_use.parallel

---

## General Notes & Constraints

- Working directory: The assistant may only read/write under `/Users/art/Documents/singularityio` and its subpaths. File tool calls must use paths under that directory.
- CLI execution: `run_cli` executes commands without a shell. Do not pass shell metacharacters, pipelines, or redirects. Use a binary name and args array.
- Web research: `web_search` and `web_fetch` are available. When using web results, only cite URLs you successfully fetched with `web_fetch`.
- Parallel execution: `multi_tool_use.parallel` can run multiple tools concurrently when the tasks are independent and can run in parallel.
- Specialists: `specialists_infer` exposes named specialists (e.g., `software_engineer`) for domain-specific inference tasks.

---

## Tool Reference

### 1) run_cli
- Namespace: functions
- Purpose: Execute a CLI command in the allowed working directory (no shell, no absolute paths).
- Important constraints:
  - Provide `command` (binary name) and `args` (array of arguments).
  - No shell pipelines, redirects, or interactive prompts.
  - `timeout_seconds` optional.
- Example:

```json
{ "command": "git", "args": ["status"], "timeout_seconds": 20 }
```


### 2) web_search
- Purpose: Search the web (SearXNG) and return top result links.
- Key params: `query`, optional `category` and `max_results`.
- Example:

```json
{ "query": "Go 1.24 http routing features", "max_results": 5 }
```


### 3) web_fetch
- Purpose: Fetch a web URL and return extracted/cleaned markdown or readable content.
- Key params: `url`, optional `max_bytes`, `prefer_readable`, `timeout_seconds`.
- Usage note: Use this to fetch pages you will cite.

```json
{ "url": "https://example.com", "prefer_readable": true, "timeout_seconds": 10 }
```


### 4) search_index
- Purpose: Index text documents for full-text search.
- Params: `id` (string), `text` (string).

### 5) search_query
- Purpose: Query the full-text index.
- Params: `query` (string), optional `limit` (int).

### 6) search_remove
- Purpose: Remove a document from the full-text index.
- Params: `id` (string)


### 7) vector_upsert
- Purpose: Upsert a vector embedding with metadata (id + vector required).
- Params: `id` (string), `vector` (number[])

### 8) vector_query
- Purpose: Query nearest neighbors using a query vector.
- Params: `vector` (number[]), optional `k` (int)

### 9) vector_delete
- Purpose: Delete a vector by id.
- Params: `id` (string)


### 10) graph_upsert_node
- Purpose: Create or update a node in a graph database.
- Params: `id` (string), optional `labels` (string[])

### 11) graph_upsert_edge
- Purpose: Create or update a relationship between nodes.
- Params: `src` (string), `dst` (string), `rel` (string)

### 12) graph_neighbors
- Purpose: List outbound neighbor IDs for a node by relationship type.
- Params: `id` (string), `rel` (string)

### 13) graph_get_node
- Purpose: Fetch a graph node by ID.
- Params: `id` (string)


### 14) llm_transform
- Purpose: Use the LLM to transform input text according to an instruction (summarize, rewrite, extract, etc.).
- Params: `instruction` (string), `input` (string), optional `model` and `system`.
- Example:

```json
{ "instruction": "Summarize the input in 3 bullets", "input": "Long article text..." }
```


### 15) describe_image
- Purpose: Describe an image file located under the locked working directory. The image is sent to the LLM as an inline data URL.
- Params: `path` (relative path under WORKDIR), optional `prompt`, `model`.
- Example:

```json
{ "path": "images/screenshot.png", "prompt": "List visible UI components" }
```


### 16) specialists_infer
- Purpose: Invoke a configured specialist for domain-specific tasks.
- Params: `specialist` (string, e.g., "software_engineer"), `prompt` (string), optional `override_reasoning_effort` ("low" | "medium" | "high").
- Example:

```json
{ "specialist": "software_engineer", "prompt": "Review this Go function for concurrency bugs" }
```


### File-system oriented tools (mcp_filesystem_*)
Note: All paths must be inside /Users/art/Documents/singularityio.

- mcp_filesystem_copy_file
  - Purpose: Copy files and directories.
  - Params: `source`, `destination`.

- mcp_filesystem_create_directory
  - Purpose: Create a directory (or ensure it exists).
  - Params: `path`.

- mcp_filesystem_get_file_info
  - Purpose: Retrieve metadata about a file or directory.
  - Params: `path`.

- mcp_filesystem_list_allowed_directories
  - Purpose: Returns list of directories the server may access.
  - Params: none.

- mcp_filesystem_list_directory
  - Purpose: List files and directories in a path.
  - Params: `path`.

- mcp_filesystem_move_file
  - Purpose: Move or rename files/directories.
  - Params: `source`, `destination`.

- mcp_filesystem_read_file
  - Purpose: Read complete file contents.
  - Params: `path`.

- mcp_filesystem_read_multiple_files
  - Purpose: Read multiple files in one call.
  - Params: `paths` (string[]).

- mcp_filesystem_search_files
  - Purpose: Recursively search for files matching a pattern.
  - Params: `path` (starting path), `pattern` (glob or substring).

- mcp_filesystem_tree
  - Purpose: Returns hierarchical JSON representation of a directory structure.
  - Params: `path`, optional `depth` (default 3), `follow_symlinks`.

- mcp_filesystem_write_file
  - Purpose: Create or overwrite a file with content.
  - Params: `path`, `content`.
  - Example:

```json
{ "path": "/Users/art/Documents/singularityio/notes.txt", "content": "Hello world" }
```


### 17) multi_tool_use.parallel
- Purpose: Run multiple tools in parallel. Useful for independent tasks that can be executed concurrently.
- Params: `tool_uses` — an array of objects with `recipient_name` (e.g., "functions.run_cli") and `parameters` (object) for each tool.
- Important: Only functions tools are permitted here. Use when tasks do not depend on each other's outputs.

Example:

```json
{
  "tool_uses": [
    { "recipient_name": "functions.web_search", "parameters": { "query": "Go modules tutorial" } },
    { "recipient_name": "functions.web_search", "parameters": { "query": "best practices go testing" } }
  ]
}
```

---

## Example Workflow Patterns

1) Fetch-and-cite web research
   - Use `web_search` to find candidate pages.
   - Use `web_fetch` to fetch each page you plan to cite.
   - Only cite URLs fetched successfully.

2) Local-code operations
   - Use `mcp_filesystem_read_file` to read files under /Users/art/Documents/singularityio.
   - Use `run_cli` to run local CLI tools (e.g., `go test`, `gofmt`) with safe args.

3) Parallel indexing
   - Use `multi_tool_use.parallel` to upload multiple documents via `search_index` or `vector_upsert` concurrently.

---

## Contact / Troubleshooting

If a tool call fails, inspect the error message returned by the tool. Common issues:
- Path not under allowed directory.
- `run_cli` used with shell tokens or interactive commands.
- `web_fetch` timeout or remote site blocking requests.

---

(Generated on behalf of the user by the assistant.)
