# Manifold Deployment Guide

This guide describes the current deployment model for Manifold as shipped in this repository.

Manifold currently stores projects on the local filesystem under the `workdir` configured in `config.yaml`.

## Deployment Modes

### Recommended: Docker Compose

For a fresh clone, the supported first-run path is:

- `pg-manifold` for PostgreSQL
- `manifold` for the Manifold server and the embedded frontend

Optional services can be added later:

- `keycloak-db` and `keycloak` for authentication testing
- `clickhouse` and `otel-collector` for persistent observability

The default dashboard does not require ClickHouse. When `obs.local.enabled` is true, Manifold serves metrics, logs, and traces from bounded process-local buffers.

### Local Host Builds

Local host builds are supported through the Makefile, but they are a developer workflow, not the simplest deployment path. Use them when you want to:

- iterate on Go code with `make build-agentd` or `make build-manifold`; both server builds write `dist/manifold`
- run the frontend separately with `pnpm -C web/agentd-ui dev`

Host builds can also run with embedded Postgres. In that mode Manifold starts a bundled PostgreSQL process during server startup and no external database service is required.

## Prerequisites

Required for Docker deployment:

- Docker with Docker Compose support
- A valid LLM API key or a reachable OpenAI-compatible endpoint
- A writable absolute host path for `workdir`

Required only for local development outside Docker:

- Go 1.26.3
- Node 22
- `pnpm`
- Chrome or another Chromium-compatible browser for browser-driven tools

## First Run

1. Copy the templates:

```bash
cp example.env .env
cp config.yaml.example config.yaml
cp specialists.yaml.example specialists.yaml
cp mcp.yaml.example mcp.yaml
mkdir -p ./tmp/manifold-workdir
```

1. Edit `config.yaml` and set the main runtime values. At minimum, ensure:

```yaml
workdir: /absolute/path/to/your/manifold/tmp/manifold-workdir

llm_client:
  provider: openai
  openai:
    apiKey: "${OPENAI_API_KEY}"

databases:
  defaultDSN: "${DATABASE_URL}"
```

1. Edit `.env` and set the secret values referenced by your YAML:

```dotenv
OPENAI_API_KEY="your_real_api_key"
DATABASE_URL="postgres://manifold:manifold@pg-manifold:5432/manifold?sslmode=disable"
CLICKHOUSE_DSN=""
```

1. Start the required services:

```bash
docker compose up -d pg-manifold manifold
```

1. Open the UI at <http://localhost:32180>.

## Self-Contained Host Deployment

This mode runs without Compose services, external Postgres, ClickHouse, or an OpenTelemetry Collector. It is useful for local single-machine installs and development environments where you want durable Manifold state without managing a separate database server.

Prerequisites:

- Go toolchain for building `manifold`
- An LLM provider, either remote or local OpenAI-compatible

Build:

```bash
make build-manifold
```

Set `.env` values so no external database or telemetry service is selected:

```dotenv
DATABASE_URL=""
CLICKHOUSE_DSN=""
OPENAI_API_KEY="your_real_api_key"
```

Configure `config.yaml`:

```yaml
databases:
  embedded: true
  embeddedPort: 5433
  embeddedDataDir: ""
  defaultDSN: ""
  chat:
    backend: auto
    dsn: ""
  search:
    backend: auto
    dsn: ""
  vector:
    backend: auto
    dsn: ""
  graph:
    backend: auto
    dsn: ""

obs:
  otlp: ""
  local:
    enabled: true
  clickhouse:
    dsn: ""
```

Run:

```bash
./dist/manifold
```

When `databases.embedded` is true, Manifold starts bundled PostgreSQL 17 before initializing stores, sets the runtime default DSN internally, and stops the embedded process during shutdown. The default data directory is `~/.manifold/embedded-postgres`; set `embeddedDataDir` to choose a different persistent location. Release builds extract the native runtime to the Manifold runtime cache and verify every runtime file checksum before executing PostgreSQL.

The embedded mode requires `pgvector`, `postgis`, and `pgrouting`. Startup fails if any required extension cannot be created or probed with `postgis_full_version()` and `pgr_version()`. Development builds without an embedded runtime can opt into the legacy downloaded/system fallback with `databases.embeddedAllowExternalRuntimeResolution: true`; release builds should leave that setting false.

## Service Map

Core services:

- `manifold`: the main Manifold container with the embedded web UI
- `pg-manifold`: PostgreSQL with pgvector, PostGIS, and pgRouting

Optional services:

- `clickhouse`: optional persistent metrics, traces, and logs query backend
- `otel-collector`: optional OTLP ingestion pipeline
- `keycloak-db`: Postgres for Keycloak
- `keycloak`: local OIDC provider for auth testing

## Ports

- `32180`: Manifold UI and API
- `5433`: PostgreSQL exposed on the host
- `8083`: Keycloak admin and auth UI when enabled
- `8123` and `9000`: ClickHouse HTTP and native ports when enabled
- `4417` and `4418`: OTLP collector ports when enabled

Inside the compose network, Manifold connects to Postgres at `pg-manifold:5432`. Host tools should use `localhost:5433`.

## Configuration Notes

- `config.yaml` is the primary runtime configuration file. `.env` is only used to supply values for `${VAR}` interpolation inside YAML.
- The runtime validates that `workdir` exists and is a directory.
- `config.yaml.example` is the full runtime reference. `specialists.yaml.example` and `mcp.yaml.example` document the optional external specialist and MCP config files.
- `databases.defaultDSN` and the per-subsystem DSNs in `config.yaml.example` are wired to `${DATABASE_URL}` for the compose network.
- For embedded Postgres, set `databases.embedded: true` and leave `DATABASE_URL`, `databases.defaultDSN`, and per-subsystem DSNs empty so Manifold can supply the embedded DSN at runtime.
- `obs.local.enabled: true` gives the dashboard local process telemetry without ClickHouse or an OpenTelemetry Collector. `obs.clickhouse.dsn` upgrades the dashboard to persistent telemetry when ClickHouse is available.
- If you use `llm_client.openai.api: responses`, you can tune context-management behavior through `llm_client.openai.extraParams` in [config.yaml.example](../config.yaml.example).
- Voice input requires an OpenAI-compatible transcription endpoint through the `stt` section in [config.yaml.example](../config.yaml.example).

## Storage Model

Projects are stored directly on disk under:

```text
$workdir/users/<user-id>/projects/<project-id>
```

Metadata is stored at:

```text
$workdir/users/<user-id>/projects/<project-id>/.meta/project.json
```

See [storage.md](./storage.md) for details.

## Auth And Observability

- Authentication is optional and disabled by default. See [auth.md](./auth.md).
- Local dashboard observability is enabled by default through process-local buffers. ClickHouse and OTLP export are optional for persistent or external observability. See [observability.md](./observability.md).

## Backup And Recovery

Back up:

- PostgreSQL data from `pg-manifold`
- embedded Postgres data from `embeddedDataDir` or `~/.manifold/embedded-postgres` when `databases.embedded` is enabled
- the entire `workdir`

At minimum, a recoverable local deployment needs both the database state and the project filesystem.
