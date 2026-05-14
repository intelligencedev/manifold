# Manifold Wiki Configuration

**Repository:** `repos/manifold`  
**Wiki root:** `repos/manifold/wiki`  
**Created:** 2026-05-14  
**Audience:** new Manifold contributors, maintainers, and LLM coding agents.

## Scope

This wiki documents the Manifold software repository, not a deployed instance. It is grounded in repository files: Go backend code, Vue frontend code, configuration examples, Docker/deploy assets, tests, and existing documentation.

## Repository Snapshot

- Backend language: Go (`go.mod` declares Go 1.25.0).
- Frontend stack: Vue 3, Vite, TypeScript, Pinia, Vue Router, Tailwind, Vue Flow.
- Main backend binary: `cmd/agentd/main.go`.
- CLI agent binary: `cmd/agent/main.go`.
- OpenAPI generator: `cmd/openapi/main.go`.
- Runtime config: `config.yaml`, `config.yaml.example`, `mcp.yaml`, `specialists.yaml.example`, `example.env`.
- Docker entry: `docker-compose.yml`, `deploy/docker/cpu.Dockerfile`, `deploy/docker/postgres.Dockerfile`.

## Diagram Standards

Use Mermaid diagram types according to what is being explained:

- `flowchart` for architecture, dependencies, pipelines, and component relationships.
- `sequenceDiagram` for request handling, CLI/runtime execution, integrations, and ordered interactions.
- `erDiagram` for persistent data relationships.
- `stateDiagram-v2` for lifecycles, run states, and retry/failure transitions.
- `classDiagram` for Go interfaces or object models where type relationships matter.

Every substantial diagram should name real repository concepts and cite evidence in the page's Evidence section.

## Evidence Policy

Important claims should be traceable to:

- repository paths and symbols;
- command output collected during the wiki build;
- existing docs in `README.md`, `QUICKSTART.md`, and `docs/`;
- tests that assert behavior.

Mark unsupported conclusions as **Inference** and important missing information as **Unknown**.

## Maintenance Checklist

When code changes substantially:

1. Update `index.md` if onboarding paths or page links change.
2. Update pages under `architecture/` when component boundaries or runtime dependencies change.
3. Update component pages when important packages, handlers, or tools change.
4. Update `interfaces/http-api.md` when `internal/agentd/router.go` or `internal/apidocs/spec.go` changes.
5. Update `data/postgres-schema.md` when SQL schema initialization or migrations change.
6. Update `development/testing.md` when CI, Makefile targets, or frontend scripts change.
7. Add an entry to `logs/maintenance-log.md`.
