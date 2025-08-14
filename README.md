# SingularityIO

An agentic CLI and TUI that uses OpenAI’s official Go SDK (v2) for chat-based tool calling. It executes commands safely in a locked working directory, supports streaming, and integrates observability (logs, traces, metrics).

Contents
- Features
- Requirements
- Quick start
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
- Specialists and routing
- Observability (logging, tracing, metrics)
- Security considerations
- Development
- Docker

Features
- OpenAI Go SDK v2 for chat completions and streaming
- Tool calling with a secure executor (no shell) in a locked WORKDIR
- Web search and fetch tools; file write tool; LLM transform helper
- Optional “specialists” (OpenAI-compatible endpoints) with simple routing
- Model Context Protocol (MCP) client that exposes server tools to the agent
- Structured JSON logging with redaction and optional payload logging
- OpenTelemetry traces and metrics; instrumented HTTP client

Requirements
- Go 1.21+
- An OpenAI API key

Quick start
1) Create a working directory and .env
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

2) Run the CLI
```
go run ./cmd/agent -q "List files and print README.md if present"
```

3) Run the TUI (streaming)
```
go run ./cmd/agent-tui
```

Configuration
Environment variables (.env)
- Place variables in a .env file at repo root; .env values override OS env.
- Common variables:
  - OPENAI_API_KEY, OPENAI_MODEL, WORKDIR
  - LOG_PATH, LOG_LEVEL, LOG_PAYLOADS
  - BLOCK_BINARIES, MAX_COMMAND_SECONDS, OUTPUT_TRUNCATE_BYTES
  - SEARXNG_URL (for web search)
  - OTEL_SERVICE_NAME, SERVICE_VERSION, ENVIRONMENT, OTEL_EXPORTER_OTLP_ENDPOINT

YAML (config.yaml)
Optionally configure richer settings in config.yaml (or set SPECIALISTS_CONFIG). Example:
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
- .env > OS environment > config.yaml. Defaults are applied only when none provide values.

Running
CLI
```
go run ./cmd/agent -q "Initialize a new module and run go test" [-max-steps 8]
```
Flags:
- -q: user request (required)
- -max-steps: limit agent steps (default 8)
- -specialist: invoke a configured specialist directly (inference-only)
- -warpp: run WARPP workflow executor instead of LLM loop

TUI (streaming)
```
go run ./cmd/agent-tui
```
Notes:
- Live streaming of assistant responses
- Same safety controls as CLI (locked WORKDIR, sanitization, blocklist)

WARPP mode
- Intent detection → personalization → fulfillment with tool allow-listing
- Uses existing tools; comes with minimalist defaults when workflows are absent

Tools
Built-in tools registered by default:
- run_cli: execute a binary (no shell) in WORKDIR with sanitized args
- web_search: query SearXNG (set SEARXNG_URL)
- web_fetch: fetch a URL
- write_file: write files to WORKDIR
- llm_transform: call LLM on a specific baseURL (helper)

MCP client
Configure one or more MCP servers via config.yaml (see example above). On startup, the client connects to each server via stdio/command, lists available tools, and registers them into the agent.

- Tools are exposed with names of the form mcp_<server>_<tool> to avoid collisions.
- Input schemas are normalized for OpenAI tool-calling compatibility.
- Optional keepAliveSeconds can be used to keep sessions active (0 disables keepalive).

Specialists and routing
Define “specialists” that target OpenAI-compatible endpoints/models. Example:
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
- Direct: `go run ./cmd/agent -specialist code-reviewer -q "Review this function"`
- Pre-dispatch: the router matches input against routes and invokes a specialist automatically.

Observability
Logging
- Structured JSON logging via zerolog
- LOG_PATH writes only to file (not stdout). This prevents TUI interference.
- LOG_LEVEL: trace | debug | info | warn | error | fatal | panic (default: info)
- LOG_PAYLOADS=false by default. If true, logs redacted payloads for:
  - Tool arguments/results
  - Provider extras sent with OpenAI requests
- Redaction masks common sensitive keys (api keys, tokens, authorization, password, secret, etc.).
- openai.logPayloads in YAML also enables payload logging; env overrides YAML.

Tracing and metrics (OpenTelemetry)
- Configure via env (e.g., OTEL_EXPORTER_OTLP_ENDPOINT)
- HTTP client is instrumented; logs include trace_id/span_id when available

Security considerations
- No shell execution; only bare binary names (no paths)
- All commands run under WORKDIR
- Optional blocklist for dangerous binaries
- Output truncation to reduce token costs

Development
- Build
  - `go build ./...`
- Test
  - `go test ./...`
- Lint/Vet
  - `go vet ./...`
- Go version: 1.21+

Docker
- Build
  - `docker build -t agent-tui .`
- Run (ensure a TTY and pass any volumes/env)
  - `docker run -it --env-file .env agent-tui`
