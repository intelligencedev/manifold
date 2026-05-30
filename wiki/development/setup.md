# Development Setup

This page describes the contributor setup path. It avoids hard-coded machine-specific paths; set local paths according to your environment.

## Required for Docker Quick Start

- Docker with Compose support.
- An LLM API key or reachable OpenAI-compatible endpoint.
- A writable host directory for Manifold project workspaces.

## Required for Host Development

- Go matching the module expectation in `go.mod`.
- Node 22 and `pnpm` for frontend development.
- Docker if using compose services or Docker-backed MCP tools.
- Chromium-compatible browser if using browser-driven tools or Playwright.

## First Local Files

From the repository root:

```bash
cp example.env .env
cp config.yaml.example config.yaml
cp mcp.yaml.example mcp.yaml
cp specialists.yaml.example specialists.yaml
```

Then edit `.env` and `config.yaml` for your provider, workdir, database mode, and optional services.

## Minimal Read List

- `README.md`
- `QUICKSTART.md`
- `docs/deployment.md`
- `config.yaml.example`
- [Architecture Overview](../architecture/overview.md)
- [Change Risk Map](change-risk-map.md)

## Frontend Setup

```bash
pnpm -C web/agentd-ui install
pnpm -C web/agentd-ui dev
```

The frontend dev server expects a running `agentd` for API calls.

## Backend Setup

Use Docker Compose for the simplest full stack, or host builds for backend iteration.

```bash
docker compose up -d pg-manifold manifold
```

For host builds:

```bash
make build-agentd
```

## Evidence

- `README.md`, `QUICKSTART.md`, `docs/deployment.md`.
- `go.mod`, `web/agentd-ui/package.json`, `web/agentd-ui/README.md`.
- `docker-compose.yml`.
