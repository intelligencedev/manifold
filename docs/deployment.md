# Manifold Deployment Guide

This guide describes the current deployment model for Manifold as shipped in this repository.

Manifold currently stores projects on the local filesystem under the `workdir` configured in `config.yaml`.

## Deployment Modes

### Recommended: Docker Compose

For a fresh clone, the supported first-run path is:

- `pg-manifold` for PostgreSQL
- `manifold` for `agentd` and the embedded frontend

Optional services can be added later:

- `keycloak-db` and `keycloak` for authentication testing
- `clickhouse` and `otel-collector` for observability

### Local Host Builds

Local host builds are supported through the Makefile, but they are a developer workflow, not the simplest deployment path. Use them when you want to:

- iterate on Go code with `make build-agentd` or `make build-manifold`
- run the frontend separately with `pnpm -C web/agentd-ui dev`
- build `manibot` directly on the host

## Prerequisites

Required for Docker deployment:

- Docker with Docker Compose support
- A valid LLM API key or a reachable OpenAI-compatible endpoint
- A writable absolute host path for `workdir`

Required only for local development outside Docker:

- Go 1.25
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
```

1. Start the required services:

```bash
docker compose up -d pg-manifold manifold
```

1. Open the UI at <http://localhost:32180>.

## Service Map

Core services:

- `manifold`: the main `agentd` container with the embedded web UI
- `pg-manifold`: PostgreSQL with pgvector, PostGIS, and pgRouting

Optional services:

- `clickhouse`: metrics, traces, and logs query backend
- `otel-collector`: OTLP ingestion pipeline
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
- Observability is optional and disabled unless you start the extra services and configure OTLP or ClickHouse. See [observability.md](./observability.md).

## Backup And Recovery

Back up:

- PostgreSQL data from `pg-manifold`
- the entire `workdir`

At minimum, a recoverable local deployment needs both the database state and the project filesystem.
