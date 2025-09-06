<p align="center">
<img src="assets/singularityio-logo.svg" alt="SingularityIO logo" width="200" />
</p>

# intelligence.dev

An agentic CLI and TUI.

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
  - embedctl
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
SingularityIO is a safe, observable agent runtime for driving tool-calling workflows using OpenAI API compatible endpoints. It restricts execution to a locked WORKDIR (no shell), supports streaming assistant output (TUI), logs and traces requests, and can connect to external tool providers via MCP or route requests to specialized endpoints.

Features
--------
- OpenAI Go SDK v2 for chat completions and streaming
- Tool calling with a secure executor (no shell) in a locked WORKDIR
- Built-in tools: run_cli, web_search, web_fetch, write_file, llm_transform
- Standalone embedding utility (`embedctl`) for generating text embeddings
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
# Optional embedding service (for embedctl)
EMBED_API_KEY=sk-...
EMBED_MODEL=text-embedding-3-small
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
  - EMBED_BASE_URL, EMBED_MODEL, EMBED_API_KEY, EMBED_API_HEADER, EMBED_PATH, EMBED_TIMEOUT (for embeddings)
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

embedding:
  baseURL: https://api.openai.com
  model: text-embedding-3-small
  apiKey: ${EMBED_API_KEY}
  apiHeader: Authorization
  path: /v1/embeddings
  timeoutSeconds: 30

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

## Run agentd and web UI (local)

Quick steps to run the HTTP agent server (`agentd`) and the embedded web UI used for manual testing.

- Prepare a `.env` at the repository root. The project will automatically load `.env` (or `example.env`) on startup. Typical entries:

```env
WORKDIR=./sandbox
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o-mini
# Optional: WEB UI settings
WEB_UI_PORT=8081
WEB_UI_HOST=0.0.0.0
WEB_UI_BACKEND_URL=http://localhost:32180/agent/run
```

- Quick dev mode (mock LLM): to test the UI without calling OpenAI, set `OPENAI_API_KEY=` (empty) in `.env` and restart `agentd` — the server will return a deterministic dev response to POST /agent/run.

- Start the agent HTTP server (`agentd`) — by default it listens on port `:32180`:

```bash
# run directly (reads .env)
go run ./cmd/agentd/main.go

# or build and run the binary
go build -o build/agentd ./cmd/agentd
./build/agentd
```

- Start the web UI (defaults to host `0.0.0.0` port `8081` and forwards prompts to the agent_backend):

```bash
# run directly
go run ./cmd/webui

# or build and run
go build -o build/webui ./cmd/webui
./build/webui
```

- Quick smoke tests (from your machine):

```bash
# fetch the web UI index
curl http://localhost:8081/

# submit a prompt (JSON forwarded to agentd)
curl -i -X POST http://localhost:8081/api/prompt \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"hello from webui"}'
```

Notes:
- `agentd` will return HTTP 500 if the configured LLM provider returns an error (for example, an unsupported parameter). For local UI testing, use the mock dev mode above or ensure your OpenAI-compatible provider and model accept the request parameters.
- If port `32180` is already in use, stop the conflicting process or run `agentd` in an environment where that port is free. (You can also run the web UI with `WEB_UI_BACKEND_URL` pointing to a different agent endpoint.)

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

Tool allow-listing
------------------
You can selectively expose a subset of registered tools to the main orchestrator agent
or to an individual specialist. This is useful to restrict tool access for safety or to
reduce the tool schema sent to models.

Top-level (main agent) allow-list:
- In `config.yaml` add `allowTools` at the top level (list of tool names):

```
allowTools:
  - run_cli
  - web_search
  - web_fetch
```

Per-specialist allow-list:
- Each `specialist` entry supports `allowTools` to limit which tools that specialist
  can see and call. If `enableTools` is false for a specialist, no tool schema is sent
  regardless of `allowTools`.

Example specialist with allow-list:

```
specialists:
  - name: data-extractor
    baseURL: https://api.openai.com
    apiKey: ${OPENAI_API_KEY}
    model: gpt-4o
    enableTools: true
    allowTools:
      - web_fetch
      - llm_transform
    system: |
      You extract structured information from text.
```

Notes:
- If an allow-list is empty or omitted, all registered tools are exposed (subject to `enableTools`).
- The MCP client registers external tools with names like `mcp_<server>_<tool>`; include those names
  in allow-lists if you want to grant access to MCP-provided tools.

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

whisper-go
--------
```
cd /Users/art/Documents/singularityio && ./run-whisper-go.sh -model /Users/art/Documents/code/whisper.cpp/bindings/go/models/ggml-small.en.bin /Users/art/Documents/singularityio/54521110-ad38-4885-b8c3-3b43bb1f4853.wav
```

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

### Build

SingularityIO uses a comprehensive build system that includes Whisper.cpp for speech-to-text functionality in the TUI.

#### Quick Build (Recommended)

```bash
# Build all binaries with Whisper support
make build

# Or build individual components
make whisper-cpp        # Build Whisper.cpp library
make whisper-go-bindings # Build Go bindings
```

#### Manual Build

If you prefer manual control:

1. **Build Whisper.cpp library:**

   ```bash
   cd external/whisper.cpp
   make build
   ```

2. **Build Go bindings:**

   ```bash
   cd external/whisper.cpp/bindings/go
   make
   ```

3. **Build Go binaries with proper environment:**

   ```bash
   C_INCLUDE_PATH=external/whisper.cpp/include \
   LIBRARY_PATH=external/whisper.cpp/build_go/src:external/whisper.cpp/build_go/ggml/src:external/whisper.cpp/build_go/ggml/src/ggml-blas:external/whisper.cpp/build_go/ggml/src/ggml-metal \
   go build ./cmd/agent-tui
   ```

#### Available Make Targets

- `make build` - Build all binaries (includes Whisper)
- `make build-tui` - Build the TUI binary quickly (skips Whisper if already built)
- `make whisper-cpp` - Build Whisper.cpp library
- `make whisper-go-bindings` - Build Go bindings for Whisper
- `make cross` - Cross-compile for multiple platforms (CGO binaries skipped)
- `make checksums` - Generate SHA256 checksums for artifacts in dist/
- `make ci` - Run CI checks (fmt-check, imports-check, vet, lint, test)
- `make clean` - Clean all build artifacts
- `make test` - Run tests
- `make fmt` - Format code
- `make fmt-check` - Check formatting
- `make imports-check` - Check imports with goimports
- `make vet` - Run go vet
- `make lint` - Run linters
- `make tools` - Install development tools

#### Build Requirements

- Go 1.21+
- CMake (for Whisper.cpp)
- C/C++ compiler (Xcode on macOS, GCC/Clang on Linux)
- For speech-to-text: Whisper model file at `models/ggml-small.en.bin`

#### Build Notes

- The TUI binary (`agent-tui`) includes Whisper integration and will be ~50MB
- Cross-compilation skips CGO-dependent binaries like `agent-tui`
- Whisper models are not included in the build; download separately if needed

### Test

```bash
go test ./...
```

### Lint/Vet

```bash
go vet ./...
make lint  # Requires golangci-lint
```

### Go Version

- Minimum: Go 1.21+
- Recommended: Go 1.24+

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
