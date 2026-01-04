# Observability

manifold provides comprehensive observability through structured logging, OpenTelemetry tracing, and metrics.

## Logging

### Configuration

Configure logging via environment variables:

```env
LOG_PATH=./agent.log       # File path for logs
LOG_LEVEL=info            # Log level: debug, info, warn, error
LOG_PAYLOADS=false        # Whether to log request/response payloads
```

### Log Format

manifold uses structured JSON logging with the following fields:
- `timestamp`: ISO 8601 timestamp
- `level`: Log level
- `message`: Human-readable message
- `component`: Component generating the log
- `request_id`: Unique request identifier (for tracing)

### Redaction

Sensitive information is automatically redacted from logs:
- API keys are masked
- Large payloads are truncated (configurable via `OUTPUT_TRUNCATE_BYTES`)
- Personal information is filtered based on patterns

## OpenTelemetry

### Configuration

Configure OpenTelemetry via environment variables:

```env
OTEL_SERVICE_NAME=manifold
SERVICE_VERSION=1.0.0
ENVIRONMENT=production
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

### Tracing

manifold automatically instruments:
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

## Enterprise Observability Stack

For production deployments, the enterprise configuration includes a complete observability stack:

- **ClickHouse**: High-performance columnar database for traces, logs, and metrics
- **OpenTelemetry Collector**: Receives, processes, and exports telemetry data

### Deployment

The observability stack is included in `docker-compose.enterprise.yml`:

```bash
docker-compose -f docker-compose.enterprise.yml up -d
```

### Configuration Files

| File | Purpose |
|------|---------|
| `configs/otel/collector.yaml` | OTEL Collector pipelines and exporters |
| `configs/clickhouse/init-otel.sql` | ClickHouse schema for OTEL data |

### Querying Traces

```sql
-- Recent slow spans (>1 second)
SELECT 
    ServiceName,
    SpanName,
    Duration / 1000000 AS duration_ms,
    SpanAttributes
FROM otel.traces
WHERE Duration > 1000000000
ORDER BY Timestamp DESC
LIMIT 20;
```

### Querying Metrics

```sql
-- Token usage over time
SELECT 
    toStartOfHour(TimeUnix) AS hour,
    Attributes['llm.model'] AS model,
    sum(Value) AS tokens
FROM otel.metrics_sum
WHERE MetricName IN ('llm.prompt_tokens', 'llm.completion_tokens')
GROUP BY hour, model
ORDER BY hour DESC;
```

See [deployment.md](deployment.md#observability-stack) for complete setup instructions.

## Monitoring

### Health Checks

agentd provides health check endpoints:
- `GET /health`: Basic health check
- `GET /ready`: Readiness check (includes database connectivity)

### Performance Monitoring

Monitor these key areas:
1. **Response Times**: Track P95/P99 latencies
2. **Error Rates**: Monitor 4xx/5xx responses
3. **Tool Performance**: Track tool execution times
4. **Database Health**: Monitor connection pool and query performance