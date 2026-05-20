# Testing

## Backend Tests

Primary command:

```bash
make test
```

This runs:

```bash
go test -race -coverprofile=coverage.out ./...
```

Focused package tests are usually faster during development, for example:

```bash
go test ./internal/agentd
go test ./internal/agent
go test ./internal/tools/...
go test ./internal/persistence/databases
go test ./internal/rag/...
go test ./internal/flow
```

## Frontend Tests

```bash
pnpm -C web/agentd-ui test:unit
pnpm -C web/agentd-ui test:e2e
pnpm -C web/agentd-ui lint
```

## CI Checks

The Makefile `ci` target chains formatting, import checks, vet, lint, and tests:

```bash
make ci
```

The GitHub Actions workflow runs Go formatting/import/vet/lint/tests with coverage artifacts and has build/release jobs.

## Test Strategy by Change Type

| Change | Minimum tests to inspect/run |
| --- | --- |
| Router/API handler | `internal/agentd/*handler*_test.go`, OpenAPI tests, frontend API client tests if affected. |
| Agent loop/tool behavior | `internal/agent/*_test.go`, `internal/tools/*/*_test.go`. |
| Projects/files/sandbox | `internal/projects/*_test.go`, `internal/workspaces/*_test.go`, `internal/sandbox/*_test.go`, file tool tests. |
| Store/schema | Store package tests plus migration review. |
| Flow v2 | `internal/flow/compiler_test.go`, `internal/agentd/flow_v2_runtime_test.go`, frontend Flow tests if UI affected. |
| MCP | `internal/mcpclient/*_test.go`, `internal/agentd/handlers_mcp_test.go`. |
| Memory/RAG/Beliefs | `internal/transit/*_test.go`, `internal/rag/**`, `internal/agent/memory/*_test.go`, belief store tests. |
| CodeQA | `internal/codeqa/**` tests, `internal/agentd/handlers_codeqa_test.go`. |
| Frontend route/view | Vitest specs and Playwright smoke tests. |

## Notable CI Caveat

The current workflow should be reviewed against the current `go.mod` Go version before relying on it as definitive. The wiki crawl observed `go.mod` declaring Go 1.26.3 and the checked-in workflow should use the same required version.

## Evidence

- `Makefile` `test` and `ci` targets.
- `.github/workflows/ci.yml`.
- `web/agentd-ui/package.json` scripts.
- Repository test file crawl under `internal/**` and `web/agentd-ui/tests`.
