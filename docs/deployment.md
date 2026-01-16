# Manifold Deployment Guide (Local Filesystem)

This guide describes the local-only deployment of Manifold. Projects are stored on the local filesystem under `WORKDIR`.

## Prerequisites

- Docker and Docker Compose
- PostgreSQL
- OpenAI API key (or compatible LLM endpoint)

## Quick Start

1. **Clone and configure**:
   ```bash
   git clone https://github.com/intelligencedev/manifold.git
   cd manifold
   cp example.env .env
   cp config.yaml.example config.yaml
   ```

2. **Edit `.env`** with required values:
   ```env
   OPENAI_API_KEY=sk-...
   WORKDIR=/absolute/path/to/manifold-workdir
   DATABASE_URL=postgres://manifold:manifold@pg-manifold:5432/manifold?sslmode=disable
   ```

3. **Start services**:
   ```bash
   docker-compose up -d pg-manifold manifold
   ```

4. **Access the UI**:
   Open http://localhost:32180

## How Storage Works (Local Only)

Projects are stored directly on disk under:

```
$WORKDIR/users/<user-id>/projects/<project-id>
```

Metadata lives in `.meta/project.json` inside each project directory. No additional storage services are required.

## Configuration Notes

- `workdir` in [config.yaml](../config.yaml) should point to the same `WORKDIR` you set in `.env`.
- Database DSNs are required for chat history, search, vector, and graph services. Use the same Postgres instance for local development.

## Observability (Optional)

Logging and OpenTelemetry are optional in local deployments. Configure with:

```env
LOG_PATH=./agent.log
LOG_LEVEL=info
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

## Backups

To back up local deployments:

- **PostgreSQL**: use `pg_dump` on the database.
- **Projects**: back up the `WORKDIR` directory.
