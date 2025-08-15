<p align="center">
<img src="assets/singularityio-logo.svg" alt="SingularityIO logo" width="200" />
</p>

# SingularityIO

An agentic CLI and TUI that uses OpenAI’s official Go SDK (v2) for chat-based tool calling. It executes commands safely in a locked working directory, supports streaming, and integrates observability (structured logs, traces, metrics). It also supports optional specialists (alternate OpenAI-compatible endpoints) and a Model Context Protocol (MCP) client to expose external tools to the agent.

Contents
- About
- Features
- Requirements
- Quick Start
- Configuration
  - Environment variables (.env)
  - YAML (config.yaml)
  - Precedence
- Running
  - CLI
  - TUI (streaming)
  - WARPP mode
- Tools
- MCP client
- Specialists \& Routing
- Observability
- Security
- Development
- Docker
- Contributing
- License

About
-----
SingularityIO is a safe, observable agent runtime for driving tool-calling workflows using OpenAI models. It restricts execution to a locked WORKDIR (no shell), supports streaming assistant output (TUI), logs and traces requests, and can connect to external tool providers via MCP or route requests to specialized endpoints.

Features
--------
- OpenAI Go SDK v2 for chat completions and streaming
- Tool calling with a secure executor (no shell) in a locked WORKDIR
- Built-in tools: run_cli, web_search, web_fetch, write_file, llm_transform
- Optional specialists (OpenAI-compatible endpoints) and route matching
- Model Context Protocol (MCP) client to register external server tools
- Structured JSON logging with redaction and optional payload logging
- OpenTelemetry traces and metrics; instrumented HTTP client
- Safety: blocked binaries, timeouts, output truncation

Requirements
------------
- Go 1.21+
- An OpenAI API key (OPENAI_API_KEY)

Quick Start
-----------
1) Create a working directory and a `.env` file at the repository root:

```
WORKDIR=./sandbox
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
# Optional logging
LOG_PATH=./agent.log
LOG_LEVEL=info
LOG_PAYLOADS=false
# Optional runtime safety
BLOCK_BINARIES=rm,sudo
MAX_COMMAND_SECONDS=30
OUTPUT_TRUNCATE_BYTES=65536
```

2) Run the CLI:

```
go run ./cmd/agent -q "List files and print README.md if present"
```

3) Run the TUI (streaming):

```
go run ./cmd/agent-tui
```

Configuration
-------------
Configuration is supported by environment variables (preferred), a `.env` file at repo root (which overrides OS env), and an optional `config.yaml` (or environment variable `SPECIALISTS_CONFIG` pointing to YAML). The precedence is described below.

Environment variables (.env)
- Place variables in a `.env` file at the repo root; values in `.env` override OS-level environment variables.
- Common variables:
  - OPENAI_API_KEY, OPENAI_MODEL, WORKDIR
  - LOG_PATH, LOG_LEVEL, LOG_PAYLOADS
  - BLOCK_BINARIES, MAX_COMMAND_SECONDS, OUTPUT_TRUNCATE_BYTES
  - SEARXNG_URL (for web search)
  - OTEL_SERVICE_NAME, SERVICE_VERSION, ENVIRONMENT, OTEL_EXPORTER_OTLP_ENDPOINT

Example `.env` (same as Quick Start):
```
WORKDIR=./sandbox
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
# Optional logging
LOG_PATH=./agent.log
LOG_LEVEL=info
LOG_PAYLOADS=false
# Optional runtime safety
BLOCK_BINARIES=rm,sudo
MAX_COMMAND_SECONDS=30
OUTPUT_TRUNCATE_BYTES=65536
```

YAML (config.yaml)
- Optionally configure richer settings in `config.yaml` (or set `SPECIALISTS_CONFIG` to point to an alternative file). Example:

```
openai:
  extraHeaders:
    X-App-Tenant: example
  extraParams:
    temperature: 0.3
  # Log redacted payloads for tools and provider extras
  logPayloads: false

workdir: ./sandbox
outputTruncateBytes: 65536
exec:
  blockBinaries: ["rm", "sudo"]
  maxCommandSeconds: 30
obs:
  serviceName: singularityio
  environment: dev
web:
  searXNGURL: http://localhost:8080

mcp:
  servers:
    - name: local-search
      command: ./bin/mcp-server
      args: ["--port", "0"]            # stdio via exec; args optional
      env: { API_TOKEN: ${MY_TOKEN} }    # optional environment variables
      keepAliveSeconds: 30               # optional client keepalive
```

Precedence
- .env > OS environment > config.yaml
- Defaults apply only when no configuration source provides values.

Running
-------
CLI
- Basic usage:

```
go run ./cmd/agent -q "Initialize a new module and run go test" [-max-steps 8]
```

- Important flags:
  - -q: user request (required)
  - -max-steps: limit agent steps (default 8)
  - -specialist: invoke a configured specialist directly (inference-only)
  - -warpp: run WARPP workflow executor instead of the LLM loop

TUI (streaming)
```
go run ./cmd/agent-tui
```
Notes:
- Live streaming of assistant responses.
- Same safety controls as CLI (locked WORKDIR, sanitization, blocklist).

WARPP mode
- WARPP is a workflow executor pattern: Intent detection  personalization  fulfillment with tool allow-listing.
- It uses existing tools and will use minimalist defaults when workflows are absent.
- Invoke with the CLI flag `-warpp`.

Tools
-----
Built-in tools registered by default:
- run_cli: execute a binary (no shell) in WORKDIR with sanitized args
- web_search: query SearXNG (set SEARXNG_URL)
- web_fetch: fetch a URL
- write_file: write files to WORKDIR
- llm_transform: call LLM on a specific baseURL (helper to transform text)

Notes:
- run_cli forbids shell execution; only bare binary names are accepted (no paths).
- The executor sanitizes inputs and enforces timeouts and blocked-binary checks.

MCP client
----------
- Configure one or more MCP servers via `config.yaml` (see example above).
- On startup the client connects to each server via stdio/command, lists available tools, and registers them into the agent.
- Tools are exposed with names of the form `mcp_<server>_<tool>` to avoid collisions.
- Input schemas are normalized for OpenAI tool-calling compatibility.
- Optional `keepAliveSeconds` can be used to keep MCP sessions active (0 disables keepalive).

Specialists \& Routing
---------------------
Define “specialists” that target OpenAI-compatible endpoints/models and route requests to them:

Example specialists and routes:
```
specialists:
  - name: code-reviewer
    baseURL: https://api.openai.com
    apiKey: ${OPENAI_API_KEY}
    model: gpt-4o-mini
    enableTools: false
    reasoningEffort: medium
    system: |
      You are a careful code review assistant. Provide actionable feedback.

  - name: data-extractor
    baseURL: https://api.openai.com
    apiKey: ${OPENAI_API_KEY}
    model: gpt-4o
    enableTools: true
    system: |
      You extract structured information from text.

routes:
  - name: code-reviewer
    contains: ["review", "code", "lint"]
  - name: data-extractor
    contains: ["extract", "fields", "parse"]
```

Usage:
- Direct invocation:
```
go run ./cmd/agent -specialist code-reviewer -q "Review this function"
```
- Router pre-dispatch:
  - The router matches input against `routes` and invokes the appropriate specialist automatically.

Observability
-------------
Logging
- Structured JSON logging via zerolog.
- LOG_PATH writes only to file (not stdout) to avoid interfering with TUI output.
- LOG_LEVEL: trace | debug | info | warn | error | fatal | panic (default: info).
- LOG_PAYLOADS=false by default. If true, the agent logs redacted payloads for:
  - Tool arguments/results
  - Provider extras sent with OpenAI requests
- Redaction masks common sensitive keys (api keys, tokens, authorization, password, secret, etc.).
- Setting `openai.logPayloads` in YAML also enables payload logging; environment variables override YAML.

Tracing and metrics (OpenTelemetry)
- Configure OTEL via environment variables (e.g., OTEL_EXPORTER_OTLP_ENDPOINT).
- The HTTP client is instrumented; logs include trace_id/span_id when available.
- Useful envs: OTEL_SERVICE_NAME, SERVICE_VERSION, ENVIRONMENT, OTEL_EXPORTER_OTLP_ENDPOINT.

Security
--------
- No shell execution — only bare binary names allowed.
- All commands are executed under the configured WORKDIR.
- Optional blocklist for dangerous binaries (`BLOCK_BINARIES` / `exec.blockBinaries`).
- Max execution time per command (`MAX_COMMAND_SECONDS` / `exec.maxCommandSeconds`).
- Output truncation to reduce token and exposure (`OUTPUT_TRUNCATE_BYTES`).
- Sensitive payload redaction for logs. Use LOG_PAYLOADS carefully.

Development
-----------
- Build:
  - `go build ./...`
- Test:
  - `go test ./...`
- Lint/Vet:
  - `go vet ./...`
- Go version: 1.21+

Docker
------
- Build:
  - `docker build -t agent-tui .`
- Run (ensure a TTY and pass any volumes/env):
  - `docker run -it --env-file .env agent-tui`

Contributing
------------
Contributions are welcome. Suggested workflow:
- Open an issue to discuss significant changes.
- Fork the repo and create a branch per feature/bug.
- Run tests locally: `go test ./...`
- Keep changes small and focused; follow existing code style.
- Submit a pull request with a clear description and tests where applicable.

License
-------
See the LICENSE file in the repository root for licensing details (e.g., MIT).

---
