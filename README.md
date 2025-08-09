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
