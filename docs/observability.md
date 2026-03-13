# Observability

Manifold supports structured logging, OTLP export, and ClickHouse-backed metrics or trace queries.

Observability is optional for local deployment. A basic first run only needs `pg-manifold` and `manifold`.

## Logging

### Configuration

Configure logging via environment variables in `.env`:

```env
LOG_PATH=manifold.log      # File path for logs
LOG_LEVEL=info             # Log level: trace, debug, info, warn, error
LOG_PAYLOADS=false         # Whether to log request/response payloads
```

### Log Format

Manifold uses structured JSON logging with fields such as:

- `timestamp`: ISO 8601 timestamp
- `level`: Log level
- `message`: Human-readable message
- `component`: Component generating the log
- `request_id`: Unique request identifier (for tracing)

### Redaction

Sensitive information is redacted from logs where possible:

- API keys are masked
- Large payloads are truncated (configurable via `OUTPUT_TRUNCATE_BYTES`)
- Personal information is filtered based on patterns

## OpenTelemetry

### OTLP Configuration

To enable the included local observability stack, start:

```bash
docker compose up -d clickhouse otel-collector
```

The collector supports two log ingestion paths:

- Direct `agentd` runs export logs over OTLP when `OTEL_LOGS_EXPORTER=otlp` is set.
- Docker deployments can tail container `stdout` and `stderr` with the `filelog`
  receiver and the OpenTelemetry container parser.

For the included Compose deployment, the `manifold` service forces `LOG_PATH` to
empty so structured logs stay on `stdout`. To let the collector read Docker
container logs on a Linux host, set `OTEL_DOCKER_CONTAINER_LOG_DIR` before
starting Compose:

```bash
export OTEL_DOCKER_CONTAINER_LOG_DIR=/var/lib/docker/containers
docker compose up -d manifold clickhouse otel-collector
```

The collector reads whichever Docker JSON log files are mounted at that path. If
you need to limit ingestion to the `manifold` container only, mount a narrowed
directory that contains just that container's JSON log file instead of the full
Docker containers directory.

If `OTEL_DOCKER_CONTAINER_LOG_DIR` is unset, the collector still accepts OTLP
logs, traces, and metrics, but it will not see Docker-managed container logs.

Then set the corresponding `.env` values if you want the app to export telemetry:

```env
OTEL_SERVICE_NAME=manifold
SERVICE_VERSION=1.0.0
ENVIRONMENT=production
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4318
```

The included compose file exposes the collector on host ports `4417` and `4418`, but the `manifold` container should use the compose service name `otel-collector`.

### Tracing

Manifold instruments:

- HTTP requests (client and server)
- Database operations
- Tool executions
- LLM API calls

### Metrics

Key metrics collected:

- Request duration and count
- Tool execution statistics
- Error rates
- Token usage (if available from LLM provider)

Token usage details:

- **Metric names:**
  - `llm.prompt_tokens` — cumulative prompt tokens by model (OTel Int64Counter).
  - `llm.completion_tokens` — cumulative completion tokens by model (OTel Int64Counter).
  - `llm.total_tokens` — total tokens recorded as a span attribute (used in traces; not an OTel counter).

- **Span attributes (used with traces):**
  - `llm.model` — model name attached to spans.
  - `llm.prompt_preview` — truncated prompt preview (set when payload logging is enabled).
  - `llm.tools`, `llm.messages` — integer attributes set on request spans.

- **Implementation notes:**
  - The counters are created in `internal/llm/observability.go` and recorded by the LLM integration.
  - Traces and span attributes are queried (for UI "Recent Runs" and traces) via ClickHouse in `internal/agentd/traces_clickhouse.go`.
  - Config keys in `config.yaml` map these metric names for ClickHouse queries: `obs.clickhouse.promptMetricName` and `obs.clickhouse.completionMetricName`.

Use these metric names/attributes when configuring dashboards or querying telemetry backends.

## Monitoring

### Health Checks

`agentd` provides health check endpoints:

- `GET /healthz`: Basic health check
- `GET /readyz`: Readiness check

### Performance Monitoring

Monitor these key areas:

1. Response times and streaming latency
2. Error rates
3. Tool execution duration and failures
4. Database and ClickHouse query health
