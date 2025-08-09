# GPTAgent

A proof-of-concept single-binary CLI agent that executes commands in a secure, tool-calling manner using OpenAI’s official Go SDK. Inspired by WARPP-style executor-ready agents.

## Features

- Loads configuration from `.env` (see example below).
- Uses OpenAI Go SDK v2.0.1 for chat-based tool-calling.
- Exposes a `run_cli` tool: executes binaries in a locked working directory (no shell).
- Optional blocklist for dangerous binaries via `BLOCK_BINARIES`.
- Sanitizes all command arguments to prevent path traversal and restricts execution to a specified directory.
- Truncates large outputs to avoid excessive token usage.
- Never invokes a shell—no pipelines or redirects supported.

## .env Example

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

## How It Works

- The agent receives a user query and interacts with OpenAI’s chat completions API.
- When a tool call is needed, it executes the requested binary (if allowed) in the locked working directory.
- All path-like arguments are sanitized to prevent escaping the working directory.
- Outputs are truncated to a configurable byte limit.
- The agent summarizes actions and results after each tool call.

## Security Notes

- Only bare binary names are allowed (no absolute/relative paths).
- All commands run with their working directory set to `WORKDIR`.
- Blocklisted binaries can be configured to prevent dangerous operations.

## Development

- Main logic is in `main.go`.
- Requires Go 1.20+ and the OpenAI Go SDK v2.
- See comments in `main.go` for further details.

---

Let me know if you want to add more details or usage examples!
