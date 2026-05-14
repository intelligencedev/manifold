# Architecture Decisions and Inferred Design Rules

This page records decisions visible in the repository. Some are explicit in docs; others are labeled as inferences from implementation structure.

## Decision: `agentd` Is the Main Runtime Host

**Evidence:** `cmd/agentd/main.go` only calls `agentd.Run()`. `internal/agentd/app_init.go` builds the app and wires stores, tools, specialists, MCP, memory, CodeQA, playground, Matrix, and workspaces.

**Impact for contributors:** most cross-cutting backend work starts in `internal/agentd`, but durable behavior should usually live in a domain package and be composed by `agentd`.

## Decision: Project Workspaces Are Filesystem-Backed

**Evidence:** `internal/projects/service.go` creates per-user project directories, writes `.meta/project.json`, and reads/writes files directly. `docs/storage.md` and `docs/deployment.md` describe filesystem-backed project storage under configured `workdir`.

**Impact:** path containment is a security boundary. Any change to project file operations, file tools, CLI execution, archive handling, or MCP path-dependent servers must preserve sandbox constraints.

## Decision: Storage Backends Are Pluggable by Subsystem

**Evidence:** `internal/persistence/databases/factory.go` builds search, vector, graph, chat, and default stores from config. It supports memory, Postgres, Qdrant vector store, noop/disabled, and auto fallback patterns.

**Impact:** new code should depend on store interfaces, not a direct database implementation, unless it is itself a store implementation.

## Decision: Application Code Initializes Many Schemas

**Evidence:** store `Init` methods create or alter tables in `internal/persistence/databases/*`, `internal/auth/store.go`, and `internal/codeqa/store/postgres.go`. Deploy migrations also exist under `deploy/postgres/migrations`.

**Impact:** schema changes must keep application initialization and migration files aligned. Add tests for new store behavior.

## Decision: Tools Are Model-Visible Capabilities with Policy Controls

**Evidence:** `internal/tools/types.go` defines tool interfaces; `internal/tools/registry*.go` handles schema exposure and dispatch; `internal/agentd/app_init.go` registers tools; `internal/tools/discovery/*` supports auto-discovery; specialist configs can override tool exposure.

**Impact:** changing a tool changes the model's action surface. Validate JSON schemas, permission boundaries, and errors-as-data behavior.

## Decision: MCP Extends the Tool Registry

**Evidence:** `mcp.yaml.example`, `internal/mcpclient/mcpclient.go`, `internal/mcpclient/pool.go`, and `internal/agentd/handlers_mcp.go` show configured and persisted MCP servers, OAuth flows, and path-dependent per-user sessions.

**Impact:** MCP tools should be treated like untrusted extension points. Workspace path substitution and per-user lifetimes are important.

## Decision: The UI Is Embedded for Production-Style Builds

**Evidence:** `deploy/docker/cpu.Dockerfile` builds `web/agentd-ui`, copies the dist assets into `internal/webui/dist`, and builds `agentd`. `web/agentd-ui/README.md` says Docker builds handle frontend bundling and `make frontend` copies embed assets.

**Impact:** backend changes that affect UI routes or API clients may require frontend rebuilds and embedded asset checks.

## Decision: Generated OpenAPI Is a Supported API Reference

**Evidence:** `cmd/openapi/main.go`, `internal/apidocs/spec.go`, `internal/agentd/handlers_openapi.go`, and `docs/openapi.md` support runtime and generated OpenAPI docs.

**Impact:** new endpoints should update both router and OpenAPI route catalog.

## Decision: Specialist and Team Configuration Can Override Orchestrator Behavior

**Evidence:** `internal/specialists/orchestrator_config.go`, `internal/specialists/registry.go`, `internal/agentd/handlers_specialists.go`, `internal/agentd/handlers_teams.go`, and tests under `internal/agentd/*specialist*`.

**Impact:** changes to provider config, tool policy, or system prompts must be tested for default orchestrator, direct specialist calls, and team delegation.

## Decision: Memory Is Multi-Layered, Not One Store

**Evidence:** Transit service, RAG service, evolving memory manager, belief store, and policy enforcer are separate packages with separate APIs and tables.

**Impact:** do not assume all memory features share one lifecycle or persistence path. Identify the relevant layer before modifying retrieval or prompt context behavior.

## Inferred Design Rules

- Keep domain logic outside `internal/agentd` when possible; let `agentd` orchestrate.
- Prefer interface-backed stores and small services over direct SQL in handlers.
- Treat all file paths from users, tools, workflows, and MCP as untrusted until sandbox-checked.
- Treat model-visible tool descriptions and schemas as public contracts.
- Update docs/OpenAPI/tests together when changing routes or API semantics.
