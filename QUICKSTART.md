# Quick Start Deployment

This guide is for a brand-new clone of the repository.

## What You Need

- Docker with Docker Compose support
- A valid LLM API key or a reachable OpenAI-compatible endpoint
- A writable absolute path on your host machine for `WORKDIR`

You do **not** need local Node, pnpm, or Go for the default Docker deployment path.

Local development only:

- Node 20 and `pnpm` are needed if you want to run or build the frontend outside Docker.
- Go 1.25 is needed if you want to build `agentd` or `manibot` on the host.
- Chrome or another Chromium-compatible browser is recommended when using browser-driven tools from a host build.

## 1. Prepare The Repo

```bash
cp example.env .env
cp config.yaml.example config.yaml
mkdir -p ./tmp/manifold-workdir
```

## 2. Edit `.env`

Set at minimum:

```dotenv
OPENAI_API_KEY="your_real_api_key"
WORKDIR="/absolute/path/to/your/manifold/tmp/manifold-workdir"
DATABASE_URL="postgres://manifold:manifold@pg-manifold:5432/manifold?sslmode=disable"
```

Notes:

- `WORKDIR` must be an absolute path on the host.
- Because Docker Compose bind-mounts `${WORKDIR}:${WORKDIR}`, the same absolute path must exist and be accessible from Docker.
- `DATABASE_URL` should point at `pg-manifold` when running inside the compose network.

## 3. Review `config.yaml`

The example config already points its Postgres-backed services at `pg-manifold:5432` and its UI/API redirects at `http://localhost:32180`.

For a first run, you usually only need to verify:

- `auth.enabled: false`
- `databases.defaultDSN` still points to `pg-manifold:5432`
- `llm_client` matches the provider you plan to use

`WORKDIR` is loaded from the environment. You do not need to add `workdir:` to `config.yaml` unless you explicitly want YAML to provide it.

## 4. Start Manifold

```bash
docker compose up -d pg-manifold manifold
```

Watch startup if needed:

```bash
docker compose logs -f pg-manifold manifold
```

## 5. Open The UI

Open <http://localhost:32180>.

Useful endpoints:

- UI and API root: <http://localhost:32180>
- Health: <http://localhost:32180/healthz>
- Readiness: <http://localhost:32180/readyz>
- API docs: <http://localhost:32180/api-docs>

## Optional Services

The base deployment only needs `pg-manifold` and `manifold`.

Optional compose services:

- `keycloak-db` and `keycloak` for local auth testing
- `clickhouse` and `otel-collector` for observability

Example:

```bash
docker compose up -d pg-manifold manifold clickhouse otel-collector
```

## Troubleshooting

- If `manifold` exits immediately, check `docker compose logs manifold`.
- If project operations fail, verify `WORKDIR` exists on the host and is writable.
- If the UI loads but model calls fail, verify `OPENAI_API_KEY` or your provider-specific base URL and model settings.
- If the database does not connect, confirm your DSN still uses `pg-manifold:5432` inside Docker, not host port `5433`.

For more detail, see [docs/deployment.md](./docs/deployment.md), [docs/auth.md](./docs/auth.md), and [docs/observability.md](./docs/observability.md).
