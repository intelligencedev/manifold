# Interface: CLI and Make Targets

Manifold includes Go binaries and Makefile targets for development and operations.

## Go Binaries

| Binary | Entry | Purpose |
| --- | --- | --- |
| `agentd` | `cmd/agentd/main.go` | Main server with HTTP API and embedded UI. |
| `agent` | `cmd/agent/main.go` | Standalone CLI agent for prompt execution and optional direct specialist invocation. |
| `openapi` | `cmd/openapi/main.go` | Generate OpenAPI JSON from `internal/apidocs`. |

## Make Targets

| Target | Purpose |
| --- | --- |
| `make tools` | Install Go development tools such as goimports and golangci-lint. |
| `make fmt` / `make fmt-check` | Format or check Go formatting. |
| `make imports-check` | Check Go imports. |
| `make vet` | Run Go vet. |
| `make lint` | Run golangci-lint. |
| `make test` | Run Go tests with race detector and coverage. |
| `make ci` | Run fmt/imports/vet/lint/test chain. |
| `make build` | Build host binaries into `dist/`. |
| `make build-agentd` | Build only `agentd`. |
| `make build-agent` | Build only `agent`. |
| `make frontend` | Install/build frontend and copy embed assets. |
| `make build-manifold` | Build frontend plus embedded `agentd`. |
| `make build-manifold-beta` | Build embedded `agentd` with beta UI feature gate. |
| `make openapi` | Generate `docs/openapi/openapi.json`. |
| `make clean` | Remove build output and coverage. |

## Frontend Scripts

From `web/agentd-ui/package.json`:

| Script | Purpose |
| --- | --- |
| `pnpm -C web/agentd-ui dev` | Vite dev server. |
| `pnpm -C web/agentd-ui build` | Production frontend build. |
| `pnpm -C web/agentd-ui test:unit` | Vitest. |
| `pnpm -C web/agentd-ui test:e2e` | Playwright. |
| `pnpm -C web/agentd-ui lint` | ESLint. |

## Evidence

- `Makefile` target extraction.
- `cmd/agentd/main.go`, `cmd/agent/main.go`, `cmd/openapi/main.go`.
- `web/agentd-ui/package.json` and `web/agentd-ui/README.md`.
