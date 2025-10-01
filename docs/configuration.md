# Configuration

Configuration is supported by environment variables (preferred), a `.env` file at repo root (which overrides OS env), and an optional `config.yaml` (or environment variable `SPECIALISTS_CONFIG` pointing to YAML). The precedence is described below.

## Environment variables (.env)

- Place variables in a `.env` file at the repo root; values in `.env` override OS-level environment variables.
- Common variables:
  - OPENAI_API_KEY, OPENAI_MODEL, WORKDIR
  - LOG_PATH, LOG_LEVEL, LOG_PAYLOADS
  - BLOCK_BINARIES, MAX_COMMAND_SECONDS, OUTPUT_TRUNCATE_BYTES
  - AGENT_RUN_TIMEOUT_SECONDS, STREAM_RUN_TIMEOUT_SECONDS, WORKFLOW_TIMEOUT_SECONDS
  - SEARXNG_URL (for web search)
  - EMBED_BASE_URL, EMBED_MODEL, EMBED_API_KEY, EMBED_API_HEADER, EMBED_PATH, EMBED_TIMEOUT (for embeddings)
  - OTEL_SERVICE_NAME, SERVICE_VERSION, ENVIRONMENT, OTEL_EXPORTER_OTLP_ENDPOINT

Example `.env`:

```env
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

## YAML (config.yaml)

- Optionally configure richer settings in `config.yaml` (or set `SPECIALISTS_CONFIG` to point to an alternative file). Example:

```yaml
openai:
  api: completions
  # Optional: request-wide headers for the main agent
  # extraHeaders:
  #   X-App-Tenant: example
  extraParams:
    parallel_tool_calls: true
    # verbosity: high
    prompt_cache_key: singularityio-
    reasoning_effort: high

# Runtime timeout controls (seconds)
# Set to 0 to disable global deadline (recommended for long-running agents with per-tool limits):
agentRunTimeoutSeconds: 0
streamRunTimeoutSeconds: 0
workflowTimeoutSeconds: 0

databases:
  defaultDSN: "postgres://intelligence_dev:intelligence_dev@localhost:5432/manifold?sslmode=disable"
  search:
    backend: postgres
    dsn: ""
  vector:
    backend: postgres
    dsn: ""
  graph:
    backend: postgres
    dsn: ""

enableTools: true
```

### Runtime Timeouts

As of v1.30+, runtime timeout controls include:

- `agentRunTimeoutSeconds` (default 0=disabled): Controls how long an agent can run across all its iterations
- `streamRunTimeoutSeconds` (default 0=disabled): Controls how long a single stream can stay open
- `workflowTimeoutSeconds` (default 0=disabled): Controls overall timeout of workflow execution

Each request logs the applied timeout (or absence) at debug level. Disabling a timeout means long-running calls are bounded only by upstream LLM/provider constraints and client disconnects.

### Specialists Routing

If using specialists, configure `specialists` and `routing` sections in your YAML. See the [specialists documentation](specialists.md) for details.

## Precedence

Configuration precedence (highest to lowest):
- .env > OS environment > config.yaml

Where precedence matters:
- Environment variables (including .env) always take precedence over config.yaml for equivalent settings
- Missing values in higher-precedence sources fall back to lower-precedence sources
- Invalid values in higher-precedence sources cause startup errors