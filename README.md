# SingularityIO

A proof-of-concept single-binary CLI agent that executes commands in a secure, tool-calling manner using OpenAI’s official Go SDK. Inspired by WARPP-style executor-ready agents.

## Features

- Loads configuration from `.env` (see example below).
- Uses OpenAI Go SDK v2.0.1 for chat-based tool-calling.
- Exposes a `run_cli` tool: executes binaries in a locked working directory (no shell).
- Optional blocklist for dangerous binaries via `BLOCK_BINARIES`.
- Sanitizes all command arguments to prevent path traversal and restricts execution to a specified directory.
- Truncates large outputs to avoid excessive token usage.
- Never invokes a shell—no pipelines or redirects supported.

## Example Configuration

Environment variables can be set directly in the CLI or defined in a `.env` file. If duplicate environment variables are set, the OS environment takes precedence.

```
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
WORKDIR=./sandbox
BLOCK_BINARIES=rm,sudo,chown,chmod,dd,mkfs,mount,umount
MAX_COMMAND_SECONDS=30
OUTPUT_TRUNCATE_BYTES=65536
```

## Usage

```sh
go run . -q "List files and print README.md if present"
go run . -q "Initialize a new module and run go test"
```

### Interactive mode (streaming)

You can also run the agent in an interactive TUI that supports streaming responses and multiple turns.

```sh
go run . -interactive [-max-steps 8] [-v]

# Or run WARPP in the TUI (no LLM; executes a personalized workflow)
go run ./cmd/agent-tui -warpp
```

What this does:

- Opens a terminal UI (bubbletea) with an input prompt.
- Each assistant response streams token-by-token as it arrives from the API.
- If the assistant decides to call the `run_cli` tool, the command is executed in your configured `WORKDIR` and the results are appended to the conversation. The assistant then continues automatically until it reaches a final answer or `-max-steps` is hit.
- You can continue entering new prompts; the conversation history is preserved for the session.

Controls:

- Enter: submit your prompt
- Ctrl+C: exit

Notes:

- Interactive mode uses the same safety controls as one-shot mode (locked `WORKDIR`, argument sanitization, optional blocklist, output truncation).
- Streaming uses the OpenAI Go SDK v2 Chat Completions streaming API and accumulates chunks to correctly handle tool calls and final content.

- WARPP mode (-warpp):

- Implements the WARPP runtime protocol: intent detection, personalization, then fulfillment with tool allow-listing.
- Uses existing tools (`web_search`, `web_fetch`, `run_cli`) with built-in default workflows when configs/workflows is empty:
  - If your prompt looks like web research, it will search and then fetch the first result.
  - Otherwise it will simply echo your input with `run_cli`.
- Tool outputs appear in the right pane; a concise summary appears in the chat pane.
## Docker

How to use:

- Build:  
  ```sh
  docker build -t agent-tui .
  ```
- Run (ensure a TTY and pass any needed volumes/env):  
  ```sh
  docker run -it agent-tui
  ```
- Or to inject different env at runtime:  
  ```sh
  docker run -it --env-file .env agent-tui
  ```

## How It Works

- The agent receives a user query and interacts with OpenAI’s chat completions API.
- When a tool call is needed, it executes the requested binary (if allowed) in the locked working directory.
- All path-like arguments are sanitized to prevent escaping the working directory.
- Outputs are truncated to a configurable byte limit.
- The agent summarizes actions and results after each tool call.

In interactive mode:

- The chat context begins with a system prompt describing the `run_cli` tool and safety rules.
- Responses are streamed; the UI updates live. Tool calls are detected during streaming, executed, and their outputs are appended as tool messages, after which streaming continues for the assistant.

## Security Notes

- Only bare binary names are allowed (no absolute/relative paths).
- All commands run with their working directory set to `WORKDIR`.
- Blocklisted binaries can be configured to prevent dangerous operations.

## Development

- Main logic is in `main.go`.
- Requires Go 1.21+ and the OpenAI Go SDK v2.
- See comments in `main.go` for further details.

---

Let me know if you want to add more details or usage examples!

## Specialist agents (multiple OpenAI‑compatible endpoints)

You can configure any number of “specialist agents,” each bound to its own OpenAI‑compatible base URL, API key, and model. Specialists are intended for direct, inference‑only calls where you may also want to:

- Provide dedicated system instructions
- Strictly disable tool calls so no tool schema is sent at all
- Optionally include a reasoning effort hint for models that support it (e.g., low | medium | high)

Why not .env for this? While `.env` is convenient for simple, flat settings, it is not well‑suited for lists of structured entries (name, baseURL, model, flags). A small YAML file is a better fit: readable, typed, and easy to source‑control. The runtime still honors `.env` for global defaults (API key, model, etc.).

### Configuration

Create `configs/specialists.yaml` (or set `SPECIALISTS_CONFIG` to a custom path). See `configs/specialists.example.yaml` for a full example.

Example:

```
specialists:
  - name: code-reviewer
    baseURL: https://api.openai.com
    apiKey: ${OPENAI_API_KEY}
    model: gpt-4o-mini
    enableTools: false           # no tool schema will be sent at all
    reasoningEffort: medium      # optional; only sent if set
    system: |
      You are a careful code review assistant. Provide actionable feedback.

  - name: data-extractor
    baseURL: https://api.openai.com
    apiKey: ${OPENAI_API_KEY}
    model: gpt-4o
    enableTools: true            # tools will be allowed if your app uses them
    system: |
      You extract structured information from text.
```

Notes:
- If `enableTools` is false, the request omits the tools field entirely (not even an empty array), satisfying “no tool calling schema is sent.”
- If `reasoningEffort` is set, the request adds `{"reasoning": {"effort": "..."}}` via the SDK’s extra field facility. Providers that ignore it will simply proceed.
- You can override the default OpenAI `baseURL`/`apiKey`/`model` per specialist.

### Using specialists from the CLI

Call a specific specialist by name and pass your prompt with `-q`:

```
go run ./cmd/agent -specialist code-reviewer -q "Review this function for concurrency issues"
```

This path performs a direct, single‑turn completion using the specialist’s endpoint/model:

- Supplies the specialist’s system instructions as the first system message
- Disables tools entirely unless `enableTools` is true
- Adds a `reasoning.effort` hint if configured

Internally this uses a dedicated request builder that conditionally sets fields so they are omitted when not used.

- Interactive mode uses the same safety controls as one-shot mode (locked `WORKDIR`, argument sanitization, optional blocklist, output truncation).
- Streaming uses the OpenAI Go SDK v2 Chat Completions streaming API and accumulates chunks to correctly handle tool calls and final content.

- WARPP mode (-warpp):

- Implements the WARPP runtime protocol: intent detection, personalization, then fulfillment with tool allow-listing.
- Uses existing tools (`web_search`, `web_fetch`, `run_cli`) with built-in default workflows when configs/workflows is empty:
  - If your prompt looks like web research, it will search and then fetch the first result.
  - Otherwise it will simply echo your input with `run_cli`.
- Tool outputs appear in the right pane; a concise summary appears in the chat pane.

## How It Works

- The agent receives a user query and interacts with OpenAI’s chat completions API.
- When a tool call is needed, it executes the requested binary (if allowed) in the locked working directory.
- All path-like arguments are sanitized to prevent escaping the working directory.
- Outputs are truncated to a configurable byte limit.
- The agent summarizes actions and results after each tool call.

In interactive mode:

- The chat context begins with a system prompt describing the `run_cli` tool and safety rules.
- Responses are streamed; the UI updates live. Tool calls are detected during streaming, executed, and their outputs are appended as tool messages, after which streaming continues for the assistant.

## Security Notes

- Only bare binary names are allowed (no absolute/relative paths).
- All commands run with their working directory set to `WORKDIR`.
- Blocklisted binaries can be configured to prevent dangerous operations.

## Development

- Main logic is in `main.go`.
- Requires Go 1.21+ and the OpenAI Go SDK v2.
- See comments in `main.go` for further details.

### Implementation details of specialists

- Config types: `internal/config.SpecialistConfig` and `Config.Specialists`
- Loader: `internal/config/loader.go` looks for `SPECIALISTS_CONFIG` or `configs/specialists.(yaml|yml)`
- Registry: `internal/specialists` constructs one client per specialist and exposes an `Inference` method
- OpenAI client: `internal/llm/openai.Client.ChatWithOptions` lets us omit tools and attach extra request fields (e.g., reasoning)


---

Let me know if you want to add more details or usage examples!

## How It Works

- The agent receives a user query and interacts with OpenAI’s chat completions API.
- When a tool call is needed, it executes the requested binary (if allowed) in the locked working directory.
- All path-like arguments are sanitized to prevent escaping the working directory.
- Outputs are truncated to a configurable byte limit.
- The agent summarizes actions and results after each tool call.

In interactive mode:

- The chat context begins with a system prompt describing the `run_cli` tool and safety rules.
- Responses are streamed; the UI updates live. Tool calls are detected during streaming, executed, and their outputs are appended as tool messages, after which streaming continues for the assistant.

## Security Notes

- Only bare binary names are allowed (no absolute/relative paths).
- All commands run with their working directory set to `WORKDIR`.
- Blocklisted binaries can be configured to prevent dangerous operations.

## Development

- Main logic is in `main.go`.
- Requires Go 1.21+ and the OpenAI Go SDK v2.
- See comments in `main.go` for further details.

---

Let me know if you want to add more details or usage examples!
