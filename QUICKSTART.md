# Quick Start Deployment

This guide is for a brand-new clone of the repository.

## What You Need

- Docker with Docker Compose support
- A valid LLM API key or a reachable OpenAI-compatible endpoint
- A writable absolute path on your host machine for `WORKDIR`

You do **not** need local Node, pnpm, or Go for the default Docker deployment path.

Local development only:

- Node 22 and `pnpm` are needed if you want to run or build the frontend outside Docker.
- Go 1.25 is needed if you want to build `agentd` or `manibot` on the host.
- Chrome or another Chromium-compatible browser is recommended when using browser-driven tools from a host build.

If you use `nvm`, run `nvm use` from the repository root to pick up the checked-in Node version.

## Docker Compose Quick Start

Use this path when you want Compose to run both Manifold and PostgreSQL.

### 1. Prepare The Repo

```bash
cp example.env .env
cp config.yaml.example config.yaml
mkdir -p ./tmp/manifold-workdir
```

### 2. Edit `.env`

Set at minimum:

```dotenv
OPENAI_API_KEY="your_real_api_key"
WORKDIR="/absolute/path/to/your/manifold/tmp/manifold-workdir"
DATABASE_URL="postgres://manifold:manifold@pg-manifold:5432/manifold?sslmode=disable"
CLICKHOUSE_DSN=""
```

Notes:

- `WORKDIR` must be an absolute path on the host.
- Because Docker Compose bind-mounts `${WORKDIR}:${WORKDIR}`, the same absolute path must exist and be accessible from Docker.
- `DATABASE_URL` should point at `pg-manifold` when running inside the compose network.
- Leave `CLICKHOUSE_DSN` empty unless you also start the optional ClickHouse service.

### 3. Review `config.yaml`

The example config already points its Postgres-backed services at `pg-manifold:5432` and its UI/API redirects at `http://localhost:32180`.

For a first run, you usually only need to verify:

- `auth.enabled: false`
- `databases.embedded: false`
- `databases.defaultDSN` still points to `pg-manifold:5432`
- `llm_client` matches the provider you plan to use
- `obs.local.enabled: true`
- `obs.clickhouse.dsn` stays empty unless you start ClickHouse

`WORKDIR` is loaded from the environment. You do not need to add `workdir:` to `config.yaml` unless you explicitly want YAML to provide it.

### 4. Start Manifold

```bash
docker compose up -d pg-manifold manifold
```

Watch startup if needed:

```bash
docker compose logs -f pg-manifold manifold
```

### 5. Open The UI

Open <http://localhost:32180>.

Project-local skills:

- Agents load skills only from `.skills/` inside the active project root.
- Skills are not shared across projects automatically.
- If a project needs custom skills, create them under that project's `.skills/` folder.
- This keeps all agent file access relative to the selected project's root path.

Useful endpoints:

- UI and API root: <http://localhost:32180>
- Health: <http://localhost:32180/healthz>
- Readiness: <http://localhost:32180/readyz>
- API docs: <http://localhost:32180/api-docs>

## Self-Contained Host Run

Use this path when you want Manifold to run without an external Postgres, ClickHouse, or OpenTelemetry Collector. You still need either an LLM API key or a local OpenAI-compatible model endpoint.

1. Build `agentd`:

```bash
make build-manifold
```

1. In `.env`, leave database and telemetry backends empty:

```dotenv
DATABASE_URL=""
CLICKHOUSE_DSN=""
OPENAI_API_KEY="your_real_api_key"
```

1. In `config.yaml`, enable embedded Postgres and local telemetry:

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

1. Run the server:

```bash
./dist/agentd
```

The first embedded Postgres run may download and unpack PostgreSQL binaries, then it stores data under `~/.manifold/embedded-postgres` unless `embeddedDataDir` is set. Dashboard telemetry uses in-process buffers and resets when `agentd` restarts.

## Optional Services

The base Docker deployment only needs `pg-manifold` and `manifold`. A host run with `databases.embedded: true` does not need `pg-manifold`.

Optional compose services:

- `keycloak-db` and `keycloak` for local auth testing
- `clickhouse` and `otel-collector` for persistent, cross-restart observability

Example:

```bash
docker compose up -d pg-manifold manifold clickhouse otel-collector
```

## Troubleshooting

- If `manifold` exits immediately, check `docker compose logs manifold`.
- If project operations fail, verify `WORKDIR` exists on the host and is writable.
- If the UI loads but model calls fail, verify `OPENAI_API_KEY` or your provider-specific base URL and model settings.
- If the database does not connect, confirm your DSN still uses `pg-manifold:5432` inside Docker, not host port `5433`.
- For an embedded Postgres host run, confirm `DATABASE_URL` and per-store DSNs are empty so the runtime-generated embedded DSN can be used.

For more detail, see [docs/deployment.md](./docs/deployment.md), [docs/auth.md](./docs/auth.md), and [docs/observability.md](./docs/observability.md).
