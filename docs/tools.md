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
- multi_tool_use_parallel

---

## General Notes & Constraints

- Working directory: The assistant may only read/write under `/Users/art/Documents/manifold` (after you rename locally) and its subpaths when operating on repository files.
- CLI execution: `run_cli` executes commands without a shell. Do not pass shell metacharacters, pipelines, or redirects. Provide a bare binary name and an args array.
- Web research: `web_search` and `web_fetch` are available. When citing web results, only cite pages successfully fetched via `web_fetch`.
- Parallel execution: `multi_tool_use_parallel` can run multiple `functions` tools concurrently when tasks are independent.
- Specialists: `specialists_infer` exposes configured specialists (e.g., `software_engineer`) for domain-specific inference tasks.

---

## Tool Reference

### 1) run_cli
- Namespace: functions
- Purpose: Execute a CLI command in the allowed working directory (no shell, no absolute paths). Uses the project's `Executor` which enforces timeouts, output size limits, and blocked binaries.
- Parameters:
  - `command` (string) — bare binary name (e.g., `git`, `go`).
  - `args` (string[]) — command arguments.
  - `timeout_seconds` (integer, optional) — overall timeout (subject to executor max).
  - `stdin` (string, optional) — data to send on stdin.
- Constraints: arguments are sanitized against the configured working directory; some binaries may be blocked by configuration.
- Example:

```json
{ "command": "git", "args": ["status"], "timeout_seconds": 20 }
```


### 2) web_search

- Purpose: Search the web via a SearXNG instance and return top result links.
- Parameters:
  - `query` (string) — search query (required).
  - `max_results` (integer, optional) — 1..10 (default 5).
  - `category` (string, optional) — e.g., `general`, `news`, `images` (default `general`).
  - `format` (string, optional) — `json` or other (default `json`).
- Notes: The tool implements rate limiting and retries to avoid abusing remote search endpoints.


### 3) web_fetch

- Purpose: Fetch a web URL over HTTP(S) and return best-effort Markdown (readability extraction when possible).
- Parameters:
  - `url` (string) — absolute http(s) URL (required).
  - `timeout_seconds` (integer, optional) — overall request timeout.
  - `max_bytes` (integer, optional) — maximum response size to read (bytes).
  - `prefer_readable` (bool, optional) — try to extract main article content.
  - `user_agent` (string, optional) — override User-Agent header.
  - `max_redirects` (integer, optional) — max redirects to follow.
- Response: Includes status, final URL, detected content type, and extracted Markdown.


### 4) search_index

- Purpose: Index text documents into the project's full-text search store.
- Parameters:
  - `id` (string, required) — unique document ID.
  - `text` (string, required) — document contents.
  - `metadata` (object, optional) — string->string map of metadata.


### 5) search_query

- Purpose: Query the full-text index and return matching documents.
- Parameters:
  - `query` (string, required)
  - `limit` (integer, optional, default 5)


### 6) search_remove

- Purpose: Remove a document from the full-text index.
- Parameters:
  - `id` (string, required)


### 7) vector_upsert

- Purpose: Upsert a vector embedding with optional metadata. Accepts either a precomputed vector or `text` to be embedded using the configured embedding provider.
- Parameters:
  - `id` (string, required)
  - `vector` (number[], optional) — precomputed embedding.
  - `text` (string, optional) — text to embed if `vector` not provided.
  - `metadata` (object, optional)


### 8) vector_query

 
- Purpose: Query nearest neighbors using a query vector.

- Parameters:
  - `vector` (number[], required)
  - `k` (integer, optional, default 5)
  - `filter` (object, optional) — metadata filter map


### 9) vector_delete

 
- Purpose: Delete a vector by ID.

- Parameters:
  - `id` (string, required)


### 10) graph_upsert_node

 
- Purpose: Create or update a node in the graph database.

- Parameters:
  - `id` (string, required)
  - `labels` (string[], optional)
  - `props` (object, optional)


### 11) graph_upsert_edge

 
- Purpose: Create or update a relationship between two nodes.

- Parameters:
  - `src` (string, required)
  - `rel` (string, required)
  - `dst` (string, required)
  - `props` (object, optional)


### 12) graph_neighbors

 
- Purpose: List outbound neighbor IDs for a node filtered by relationship type.

- Parameters:
  - `id` (string, required)
  - `rel` (string, required)


### 13) graph_get_node

 
- Purpose: Fetch a node by ID.

- Parameters:
  - `id` (string, required)


### 14) llm_transform

- Purpose: Use the configured LLM provider to transform input text according to an instruction (summarize, rewrite, extract, synthesize, etc.).
- Parameters:
  - `instruction` (string, required)
  - `input` (string, optional)
  - `system` (string, optional) — optional system prompt to set assistant behavior
  - `model` (string, optional) — model override
  - `base_url` (string, optional) — provider base URL override
- Notes: The tool prefers an llm.Provider propagated via context so callers (agents/specialists) can have tools use the same provider/model.


### 15) describe_image
- Purpose: Describe an image file located under the locked working directory. The tool encodes the image and sends it to the LLM (data URL or provider-specific attachment).
- Parameters:
  - `path` (string, required) — relative path under the tool's configured workdir.
  - `prompt` (string, optional) — additional instruction for the model.
  - `model` (string, optional)
  - `base_url` (string, optional)
- Notes: The tool sanitizes the path against the working directory and reads the file contents prior to sending.


### 16) specialists_infer
- Purpose: Invoke a configured specialist for domain-specific inference tasks (code review, structured extraction, etc.).
- Parameters:
  - `specialist` (string, required) — name of the specialist to invoke.
  - `prompt` (string, required) — input for the specialist.
  - `override_reasoning_effort` (string, optional) — one of `low`, `medium`, `high`.


### 17) multi_tool_use_parallel
- Purpose: Run multiple functions tools in parallel. Useful for independent tasks that can run concurrently (indexing, fetches, searches).
- Parameters: `tool_uses` — an array of objects with `recipient_name` (e.g., `functions.web_search`) and `parameters` (object) for each tool.
- Important: Only tools in the `functions` namespace are permitted here. Use when tasks do not depend on each other's outputs. Older payloads that reference `multi_tool_use.parallel` continue to work as an alias.

Example:

```json
{
  "tool_uses": [
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
    - multi_tool_use_parallel

    ---

    ## General Notes & Constraints

  - Working directory: The assistant may only read/write under `/Users/art/Documents/manifold` (after you rename locally) and its subpaths when operating on repository files.
    - CLI execution: `run_cli` executes commands without a shell. Do not pass shell metacharacters, pipelines, or redirects. Provide a bare binary name and an args array.
    - Web research: `web_search` and `web_fetch` are available. When citing web results, only cite pages successfully fetched via `web_fetch`.
    - Parallel execution: `multi_tool_use_parallel` can run multiple `functions` tools concurrently when tasks are independent.
    - Specialists: `specialists_infer` exposes configured specialists (e.g., `software_engineer`) for domain-specific inference tasks.

    ---

    ## Tool Reference

    ### 1) run_cli

    - Namespace: functions

    - Purpose: Execute a CLI command in the allowed working directory (no shell, no absolute paths). Uses the project's `Executor` which enforces timeouts, output size limits, and blocked binaries.

    - Parameters:
      - `command` (string) — bare binary name (e.g., `git`, `go`).
      - `args` (string[]) — command arguments.
      - `timeout_seconds` (integer, optional) — overall timeout (subject to executor max).
      - `stdin` (string, optional) — data to send on stdin.

    - Constraints: arguments are sanitized against the configured working directory; some binaries may be blocked by configuration.

    - Example:

    ```json
    { "command": "git", "args": ["status"], "timeout_seconds": 20 }
    ```

    ### 2) web_search

    - Purpose: Search the web via a SearXNG instance and return top result links.

    - Parameters:
      - `query` (string) — search query (required).
      - `max_results` (integer, optional) — 1..10 (default 5).
      - `category` (string, optional) — e.g., `general`, `news`, `images` (default `general`).
      - `format` (string, optional) — `json` or other (default `json`).

    - Notes: The tool implements rate limiting and retries to avoid abusing remote search endpoints.

    ### 3) web_fetch

    - Purpose: Fetch a web URL over HTTP(S) and return best-effort Markdown (readability extraction when possible).

    - Parameters:
      - `url` (string) — absolute http(s) URL (required).
      - `timeout_seconds` (integer, optional) — overall request timeout.
      - `max_bytes` (integer, optional) — maximum response size to read (bytes).
      - `prefer_readable` (bool, optional) — try to extract main article content.
      - `user_agent` (string, optional) — override User-Agent header.
      - `max_redirects` (integer, optional) — max redirects to follow.

    - Response: Includes status, final URL, detected content type, and extracted Markdown.

    ### 4) search_index

    - Purpose: Index text documents into the project's full-text search store.

    - Parameters:
      - `id` (string, required) — unique document ID.
      - `text` (string, required) — document contents.
      - `metadata` (object, optional) — string->string map of metadata.

    ### 5) search_query

    - Purpose: Query the full-text index and return matching documents.

    - Parameters:
      - `query` (string, required)
      - `limit` (integer, optional, default 5)

    ### 6) search_remove

    - Purpose: Remove a document from the full-text index.

    - Parameters:
      - `id` (string, required)

    ### 7) vector_upsert

    - Purpose: Upsert a vector embedding with optional metadata. Accepts either a precomputed vector or `text` to be embedded using the configured embedding provider.

    - Parameters:
      - `id` (string, required)
      - `vector` (number[], optional) — precomputed embedding.
      - `text` (string, optional) — text to embed if `vector` not provided.
      - `metadata` (object, optional)

    ### 8) vector_query

    - Purpose: Query nearest neighbors using a query vector.

    - Parameters:
      - `vector` (number[], required)
      - `k` (integer, optional, default 5)
      - `filter` (object, optional) — metadata filter map

    ### 9) vector_delete

    - Purpose: Delete a vector by ID.

    - Parameters:
      - `id` (string, required)

    ### 10) graph_upsert_node

    - Purpose: Create or update a node in the graph database.

    - Parameters:
      - `id` (string, required)
      - `labels` (string[], optional)
      - `props` (object, optional)

    ### 11) graph_upsert_edge

    - Purpose: Create or update a relationship between two nodes.

    - Parameters:
      - `src` (string, required)
      - `rel` (string, required)
      - `dst` (string, required)
      - `props` (object, optional)

    ### 12) graph_neighbors

    - Purpose: List outbound neighbor IDs for a node filtered by relationship type.

    - Parameters:
      - `id` (string, required)
      - `rel` (string, required)

    ### 13) graph_get_node

    - Purpose: Fetch a node by ID.

    - Parameters:
      - `id` (string, required)

    ### 14) llm_transform

    - Purpose: Use the configured LLM provider to transform input text according to an instruction (summarize, rewrite, extract, synthesize, etc.).

    - Parameters:
      - `instruction` (string, required)
      - `input` (string, optional)
      - `system` (string, optional) — optional system prompt to set assistant behavior
      - `model` (string, optional) — model override
      - `base_url` (string, optional) — provider base URL override

    - Notes: The tool prefers an llm.Provider propagated via context so callers (agents/specialists) can have tools use the same provider/model.

    ### 15) describe_image

    - Purpose: Describe an image file located under the locked working directory. The tool encodes the image and sends it to the LLM (data URL or provider-specific attachment).

    - Parameters:
      - `path` (string, required) — relative path under the tool's configured workdir.
      - `prompt` (string, optional) — additional instruction for the model.
      - `model` (string, optional)
      - `base_url` (string, optional)

    - Notes: The tool sanitizes the path against the working directory and reads the file contents prior to sending.

    ### 16) specialists_infer

    - Purpose: Invoke a configured specialist for domain-specific inference tasks (code review, structured extraction, etc.).

    - Parameters:
      - `specialist` (string, required) — name of the specialist to invoke.
      - `prompt` (string, required) — input for the specialist.
      - `override_reasoning_effort` (string, optional) — one of `low`, `medium`, `high`.

    ### 17) multi_tool_use_parallel

    - Purpose: Run multiple functions tools in parallel. Useful for independent tasks that can run concurrently (indexing, fetches, searches).

    - Parameters: `tool_uses` — an array of objects with `recipient_name` (e.g., `functions.web_search`) and `parameters` (object) for each tool.

    - Important: Only tools in the `functions` namespace are permitted here. Use when tasks do not depend on each other's outputs.

    Example:

    ```json
    {
      "tool_uses": [
        { "recipient_name": "functions.web_search", "parameters": { "query": "Go modules tutorial" } },
        { "recipient_name": "functions.web_fetch", "parameters": { "url": "https://example.com/article" } }
      ]
    }
    ```

    ---

    ## Example Workflow Patterns

    1) Fetch-and-cite web research
       - Use `web_search` to discover candidate pages.
       - Use `web_fetch` to fetch pages you plan to cite and extract Markdown.
       - Only cite pages successfully fetched via `web_fetch`.

    2) Local-code operations
       - Use `run_cli` to run local CLI tools (e.g., `go test`, `gofmt`) with safe args. Remember binary blocking and timeouts.

    3) Parallel indexing
       - Use `multi_tool_use_parallel` to run multiple `search_index` or `vector_upsert` calls concurrently.

    ---

    ## Contact / Troubleshooting

    If a tool call fails, inspect the error message returned by the tool. Common issues:
    - Path not under allowed directory when reading repository files.
    - `run_cli` used with shell tokens or interactive commands.
    - `web_fetch` timeout or remote site blocking requests.

    ---

    (Generated on behalf of the user by the assistant.)
