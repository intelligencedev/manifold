# Wiki Evidence Log

This page records the repository evidence used to build the wiki. It intentionally separates direct observations from inferences.

## Repository Commands Run

| Command | Result used |
| --- | --- |
| `ls -la repos/manifold` | Confirmed repo root files: `README.md`, `QUICKSTART.md`, `Makefile`, `go.mod`, `config.yaml.example`, `docker-compose.yml`, `cmd/`, `internal/`, `web/`, `docs/`, `deploy/`. |
| `find repos/manifold -maxdepth 3 -type d` | Confirmed top-level package and app boundaries. |
| `find repos/manifold/internal -maxdepth 3 -name '*.go' -type f` | Enumerated backend packages, tests, stores, tools, handlers, runtimes. |
| `find repos/manifold/web/agentd-ui/src -maxdepth 3 -type f` | Enumerated frontend routes, stores, APIs, views, components. |
| `python3` route extraction from `internal/apidocs/spec.go` | Found 77 OpenAPI route catalog entries. |
| `python3` SQL extraction under `internal/persistence/databases`, `internal/auth`, `internal/codeqa/store` | Found SQL-backed table groups for auth, chat, projects, specialists, Flow, Transit, Code QA, Matrix, Pulse, playground, search/vector/graph, evolving memory, and belief memory. |
| `python3` Makefile target extraction | Found build/test/development targets including `test`, `ci`, `build-agentd`, `build-agent`, `build-manifold`, `frontend`, and `openapi`. |

## High-Value Source Files

| Area | Evidence | Why it matters |
| --- | --- | --- |
| Product overview | `README.md`, `QUICKSTART.md` | Product intent, deployment path, feature list, experimental warning. |
| Main server entrypoint | `cmd/agentd/main.go`, `internal/agentd/run.go`, `internal/agentd/app_init.go` | `agentd` startup, config loading, app construction, tool registration, stores, MCP, runtimes. |
| CLI entrypoints | `cmd/agent/main.go`, `cmd/openapi/main.go` | Standalone agent invocation and OpenAPI generation. |
| HTTP API | `internal/agentd/router.go`, `internal/apidocs/spec.go`, `docs/openapi.md` | Route registrations and generated OpenAPI catalog. |
| Agent loop | `internal/agent/engine.go`, `internal/agent/engine_loop.go`, `internal/agent/engine_run.go`, `internal/agent/engine_stream.go`, `internal/agent/engine_tools.go`, `internal/agent/messages.go` | LLM message construction, tool execution, streaming, delegation, summarization hooks. |
| Chat runtime | `internal/agentd/handlers_chat.go`, `internal/agentd/chat_handler_setup.go`, `internal/agentd/chat_execution.go`, `internal/agentd/chat_target_dispatch.go`, tests in `internal/agentd/*chat*_test.go` | Main chat request flow and dispatch target selection. |
| Tools | `internal/tools/**`, `internal/agentd/app_init.go` lines around tool registrations | Built-in tool catalog and registration sequence. |
| MCP | `internal/mcpclient/mcpclient.go`, `internal/mcpclient/pool.go`, `internal/agentd/handlers_mcp.go`, `mcp.yaml.example`, `docs/mcp.md` | MCP server registration, stored server management, path-dependent per-user sessions, OAuth helpers. |
| Projects and filesystem safety | `internal/projects/service.go`, `internal/workspaces/manager.go`, `internal/sandbox/pathpolicy.go`, `internal/tools/filetool/tool.go`, `docs/storage.md` | Project layout, file operations, path containment, workspace checkout. |
| Persistence | `internal/persistence/databases/factory.go`, `internal/persistence/store.go`, stores under `internal/persistence/databases/` | Store construction, memory/Postgres backends, schema initialization. |
| Flow v2 | `internal/flow/types.go`, `internal/flow/compiler.go`, `internal/agentd/handlers_flow_v2.go`, `internal/agentd/flow_v2_runtime.go`, `docs/warpp.md`, `docs/parallel-execution-plan.md` | Workflow schema, validation, runtime, events, saved workflow tools. |
| Memory | `internal/transit/*`, `internal/rag/**`, `internal/agent/memory/**`, `internal/agent/belief/**`, `internal/policy/**`, docs under `docs/transit.md` and `docs/evolving_memory.md` | Shared memory, RAG, evolving memory, belief memory, policy enforcement. |
| Specialists | `internal/specialists/*`, `internal/agentd/handlers_specialists.go`, `internal/agentd/handlers_teams.go`, `specialists.yaml.example` | Specialist registry, orchestrator config, routing, team composition. |
| Matrix and Pulse | `internal/matrixgw/*`, `internal/pulse/service.go`, `internal/agentd/matrix_*.go`, `docs/matrix-gateway.md` | Matrix sync routing and scheduled room tasks. |
| Code QA | `internal/codeqa/**`, `internal/tools/codeqa/tool.go`, `internal/agentd/handlers_codeqa.go` | Deterministic gates, LLM judge, artifacts, API/tool integration. |
| Playground | `internal/playground/**`, `web/agentd-ui/src/views/playground/*`, `docs/playground-tutorial.md` | Prompt/dataset/experiment workflow. |
| Frontend | `web/agentd-ui/package.json`, `web/agentd-ui/src/router/index.ts`, `web/agentd-ui/src/api/*`, `web/agentd-ui/src/stores/*`, `web/agentd-ui/src/App.vue` | UI routes, API clients, state, feature gates. |
| Deployment | `docker-compose.yml`, `deploy/docker/cpu.Dockerfile`, `deploy/docker/postgres.Dockerfile`, `deploy/configs/otel/collector.yaml`, `docs/deployment.md` | Docker services, embedded UI build, Postgres extensions, optional telemetry. |
| CI and tests | `.github/workflows/ci.yml`, `Makefile`, package tests under `internal/**`, `web/agentd-ui/package.json` | Developer validation commands and CI expectations. |

## Direct Observations

- `cmd/agentd/main.go` delegates directly to `agentd.Run()`.
- `internal/agentd/app_init.go` constructs the app, initializes stores, registers built-in tools, registers RAG/Transit/CodeQA tools, creates specialists, registers MCP, and builds the tool index.
- `internal/agentd/router.go` registers health/readiness, auth, projects, Matrix, CodeQA, runs/chat, Transit, users, specialists/teams, metrics, config, Flow v2, agent, prompt, media, MCP, memory, and belief debug routes.
- `internal/apidocs/spec.go` has a route catalog used to generate OpenAPI JSON.
- `internal/persistence/databases/factory.go` selects memory, Postgres, Qdrant, noop, and auto backends depending on configuration and DSN availability.
- `web/agentd-ui/src/router/index.ts` defines the frontend navigation surface: overview, projects, specialists, chat, pulse, flow, codeqa, beliefs, settings, and playground routes.

## Inferences

- **Inference:** `internal/agentd` is the integration hub because it owns app construction, route registration, runtime wiring, and feature handler orchestration.
- **Inference:** The safest contributor workflow is subsystem-first: identify the component and tests before changing shared infrastructure such as persistence, path safety, the agent loop, or configuration loading.
- **Inference:** DB schema initialization is partially application-driven rather than exclusively migration-driven, because many stores create or alter tables during `Init`.

## Unknowns / Follow-Up Checks

- Confirm whether the current GitHub Actions Go matrix is intentional, because `go.mod` declares Go 1.26.3 and the workflow should use the same required version.
- Confirm whether all OpenAPI catalog paths match the router for routes added after the catalog was last updated.
- Confirm which beta UI links should be visible in stable builds before documenting them as end-user-facing features.
- Confirm production-hardening expectations before using Manifold outside development or controlled experimental deployments.
