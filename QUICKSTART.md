# Quick Start Deployment

This guide is for a brand-new clone of the repository.

## What You Need

- Docker with Docker Compose support
- A valid LLM API key or a reachable OpenAI-compatible endpoint
- A writable absolute path on your host machine for `WORKDIR`

You do **not** need local Node, pnpm, or Go for the default Docker deployment path.

Local development only:

- Node 22 and `pnpm` are needed if you want to run or build the frontend outside Docker.
- Go 1.26.3 is needed if you want to build `manifold` on the host.
- Chrome or another Chromium-compatible browser is recommended when using browser-driven tools from a host build.

If you use `nvm`, run `nvm use` from the repository root to pick up the checked-in Node version.

## Docker Compose Quick Start

Use this path when you want Compose to run Manifold with the default SQLite backend.

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
DATABASE_URL=""
CLICKHOUSE_DSN=""
```

Notes:

- `WORKDIR` must be an absolute path on the host.
- Because Docker Compose bind-mounts `${WORKDIR}:${WORKDIR}`, the same absolute path must exist and be accessible from Docker.
- Leave `DATABASE_URL` empty for the default SQLite backend.
- Leave `CLICKHOUSE_DSN` empty unless you also start the optional ClickHouse service.

### 3. Review `config.yaml`

The example config uses SQLite at `~/.manifold/manifold.db` and points its UI/API redirects at `http://localhost:32180`.

For a first run, you usually only need to verify:

- `auth.enabled: false`
- `databases.backend: sqlite`
- `databases.defaultDSN: ""`
- `llm_client` matches the provider you plan to use
- `obs.local.enabled: true`
- `obs.clickhouse.dsn` stays empty unless you start ClickHouse

`WORKDIR` is loaded from the environment. You do not need to add `workdir:` to `config.yaml` unless you explicitly want YAML to provide it.

### 4. Start Manifold

```bash
docker compose up -d manifold
```

Watch startup if needed:

```bash
docker compose logs -f manifold
```

### 5. Open The UI

Open <http://localhost:32180>.

Project-local skills:

- Agents load project skills from `skills/` inside the active project root.
- Agents also discover universal read-only skills from `$HOME/.manifold/skills` and `$HOME/.agents/skills`.
- If a project needs custom overrides, create them under that project's `skills/` folder.
- This keeps all agent file access relative to the selected project's root path.

Useful endpoints:

- UI and API root: <http://localhost:32180>
- Health: <http://localhost:32180/healthz>
- Readiness: <http://localhost:32180/readyz>
- API docs: <http://localhost:32180/api-docs>

## Self-Contained Host Run

Use this path when you want Manifold to run without an external Postgres, ClickHouse, or OpenTelemetry Collector. You still need either an LLM API key or a local OpenAI-compatible model endpoint.

1. Build `manifold`:

```bash
make build-manifold
```

This is the standard host build. It produces `dist/manifold` with the Forge backend and embedded frontend.

1. In `.env`, leave database and telemetry backends empty:

```dotenv
DATABASE_URL=""
CLICKHOUSE_DSN=""
OPENAI_API_KEY="your_real_api_key"
```

1. In `config.yaml`, keep SQLite and local telemetry enabled:

```yaml
databases:
  backend: sqlite
  defaultDSN: ""
  sqlite:
    path: "~/.manifold/manifold.db"
  chat:
    backend: sqlite
  search:
    backend: sqlite
  vector:
    backend: sqlite
  graph:
    backend: sqlite

obs:
  otlp: ""
  local:
    enabled: true
  clickhouse:
    dsn: ""
```

1. Run the server:

```bash
./dist/manifold storage doctor --json
./dist/manifold
```

SQLite durable state is stored at `~/.manifold/manifold.db` unless `databases.sqlite.path` is set. Dashboard telemetry uses in-process buffers and resets when `manifold` restarts.

## Optional Services

The base Docker deployment only needs `manifold`. Add `pg-manifold` only when you explicitly configure `databases.backend: postgres`.

Optional compose services:

- `keycloak-db` and `keycloak` for local auth testing
- `clickhouse` and `otel-collector` for persistent, cross-restart observability

Example:

```bash
docker compose up -d manifold clickhouse otel-collector
```

## Troubleshooting

- If `manifold` exits immediately, check `docker compose logs manifold`.
- If project operations fail, verify `WORKDIR` exists on the host and is writable.
- If the UI loads but model calls fail, verify `OPENAI_API_KEY` or your provider-specific base URL and model settings.
- Run `./dist/manifold storage doctor --json` to verify SQLite, FTS5, Vec1, WAL, and a temp vector query.
- For optional Postgres, confirm your DSN uses `pg-manifold:5432` inside Docker, not host port `5433`.

For more detail, see [docs/deployment.md](./docs/deployment.md), [docs/auth.md](./docs/auth.md), and [docs/observability.md](./docs/observability.md).
