# manifold – Architecture & Module Map
## 1. What this system does
- **Purpose:** Manifold is an AI workflow automation platform. It lets users run autonomous or semi-autonomous agents, orchestrate tool-driven workflows, and observe / manage the runs through an HTTP API and web UI.
- **Key capabilities:**
  - Chat-style agent execution with tool use, specialist routing, MCP tool ingestion, and optional WARPP (workflow) execution.
  - A long-running daemon (`agentd`) that exposes REST + SSE endpoints, authentication, projects, playground/experiments, RAG ingestion, and observability.
  - A Kafka-facing orchestrator that executes WARPP workflows in response to command envelopes and publishes results.
  - Ancillary CLIs for ad-hoc agent runs, speech-to-text, and developer tooling, plus a Svelte/Vue-based frontend served from `web/agentd-ui`.

## 2. High-level architecture
- **Core loop:** Everything centers on `internal/agent.Engine`, which iteratively calls an LLM provider with tool schemas, dispatches tool calls via `internal/tools`, and keeps conversation memory summarized when thresholds are exceeded. Both the CLI and server paths ultimately drive this engine.
- **Services & processes:**
  - `cmd/agent` – single-run CLI that loads config, seeds specialists from DB/YAML, builds an LLM provider, registers tools (run_cli, web, patch, RAG, MCP, etc.), then executes the engine (or WARPP) once.
  - `cmd/agentd` – HTTP server. `internal/agentd.Run` loads config, initializes observability, databases, tool registry, MCP clients, specialists cache, WARPP runner, playground service, and HTTP router. Endpoints under `/api/*`, `/agent/*`, `/auth/*`, `/audio/*`, `/stt`, and the SPA frontend all live here.
  - `cmd/orchestrator` – Kafka worker. It loads config, constructs the same tool/LLM stack, wraps the WARPP runner via `internal/orchestrator`, consumes command messages, dedupes via Redis, and executes workflows, publishing successes/errors to response topics/DLQs.
  - `cmd/whisper-go` – thin CLI around the vendored `whisper.cpp` binding for audio transcription.
- **Supporting subsystems:**
  - `internal/config` resolves YAML/env into strongly typed structs shared by every binary.
  - `internal/llm` abstracts OpenAI-compatible, Anthropic, and Google providers and exposes streaming + tracing hooks.
  - `internal/tools` holds the dynamic registry, tool schemas, and concrete tool packages (CLI, web, RAG, patch, utility, Kafka, specialists, multi-tool wrapper, etc.).
  - `internal/persistence/databases` fabricates memory/Postgres backends for chat history, search, vector, graph, WARPP, MCP credential stores, and the playground store.
  - `internal/warpp` loads JSON workflows, applies guards/personalization, and executes named steps via the shared tool registry.
  - `internal/playground`, `internal/rag`, `internal/projects`, and `internal/webui` bolt on higher-level product features.
- **Data/control flow (simplified):**
  ```mermaid
  graph TD
    subgraph Runtime
      CFG[config.Load]
      LLM[internal/llm Provider]
      TOOLS[internal/tools.Registry]
      AGENG[internal/agent.Engine]
      DBM[databases.Manager]
    end
    CLI[cmd/agent]
    SRV[cmd/agentd]
    ORCH[cmd/orchestrator]
    UI[web/agentd-ui]
    Kafka[(Kafka)]
    MCP[(MCP Servers)]

    CLI --> CFG --> LLM
    SRV --> CFG
    ORCH --> CFG
    CFG --> DBM --> TOOLS
    LLM --> AGENG
    TOOLS --> AGENG
    SRV -->|HTTP| UI
    ORCH -->|Commands| Kafka --> ORCH
    TOOLS --> MCP
  ```
  The binaries differ mainly in how they source input (CLI flags, HTTP requests, Kafka messages) and how results are exposed (stdout, HTTP responses, Kafka responses). Internally they reuse the same modules.

## 3. Modules and packages
- **./cmd/**
  - Houses the Go entrypoints (`agent`, `agentd`, `orchestrator`, `whisper-go`, experimental `adk`). Each subdirectory has a `main.go` that wires config + internal packages for that binary.

- **./internal/agent**
  - Implements the multi-step agent engine, message building, historical summarization, WARPP helper, and shared agent prompts. Interacts with `internal/llm` for chat/stream APIs and `internal/tools` for tool dispatch.

- **./internal/agentd**
  - Web server layer. `run.go` bootstraps the app struct (config, HTTP client, DB manager, tool registries, specialists store, MCP manager, WARPP runner, projects service, whisper model, auth provider). Handler files (`handlers_chat.go`, `handlers_tools.go`, `handlers_projects.go`, etc.) expose REST endpoints; `router.go` wires them. Depends on almost every other package (auth, agent, playground, rag, projects, tools, specialists, observability, webui).

- **./internal/orchestrator**
  - Kafka integration and WARPP adapter logic. Contains the Kafka handler (`handler.go`), dedupe interfaces, kafka admin helpers, and the `NewWarppAdapter` bridging to `warpp.Runner`.

- **./internal/warpp**
  - Workflow definitions: loader, guards, runner, personalization, tool gating, and StepPublisher wiring. Used by agent CLI (WARPP mode), agentd, and orchestrator.

- **./internal/tools**
  - Registry plus concrete tool implementations under subpackages (`cli`, `web`, `patchtool`, `rag`, `llmtool`, `specialists`, `imagetool`, `kafka`, `multitool`, `tts`, `utility`, `textsplitter`, `warpptool`, etc.). Tools expose JSON schemas, get registered per-process, and are dispatched via the engine.

- **./internal/llm**
  - Provider abstraction, OpenAI/Anthropic/Google clients, streaming hooks, observability glue, and provider factory. Supplies consistent APIs for chat (sync + stream) and tool call handling.

- **./internal/specialists**
  - Registry + router for named specialists (alternate LLM endpoints). Handles DB-backed storage (`internal/persistence/databases/specialists_store.go`), YAML seeding, route matching, and tool exposure.

- **./internal/mcpclient**
  - Registers Model Context Protocol servers (stdio or streamable HTTP). Extends the tool registry with remote tools using config or DB-provisioned entries.

- **./internal/persistence**
  - Interface definitions and database implementations (memory/Postgres) for chat store, search, vector, graph, WARPP workflows, MCP servers, etc. `databases.Manager` orchestrates backend selection based on config.

- **./internal/rag**
  - Chunking, embedding, ingest, retrieve, and service orchestration for Retrieval Augmented Generation workflows. Tightly coupled with database manager for search/vector sources and used by tools exposed in the registry.

- **./internal/playground**
  - Prompt/dataset/experiment management, artifact storage, worker execution, evaluator runner, and HTTP API server (`internal/httpapi`). agentd mounts these handlers under `/api/v1/playground`.

- **./internal/projects**
  - Filesystem-backed project service with optional envelope encryption. Serves `/api/projects` endpoints for storing per-user artifacts on disk.

- **./internal/auth**
  - OIDC/OAuth2 provider integrations, session store, middleware, and RBAC helpers. agentd conditionally wraps routes with this middleware.

- **./internal/config**
  - Config structs + loader. Defines everything the binaries rely on: LLM client selection, tool toggles, MCP servers, TTS, Kafka topics, database DSNs, auth, etc.

- **./internal/observability**
  - Logger initialization, HTTP client with tracing headers, OpenTelemetry bootstrap, payload redaction utilities. Used in every binary at startup.

- **./internal/textsplitters, ./internal/embedding, ./internal/policy, ./internal/sandbox, ./internal/mesh**
  - Supporting libs: splitter strategies for RAG, HTTP embedding client, policy helpers, sandbox/workdir enforcement, and groundwork for multi-agent meshes.

- **./internal/webui**
  - Static file handler for the SPA, dev proxy wrapper, auth-gated asset serving. agentd registers it for `/` routes.

- **./docs, ./examples, ./configs, ./deploy, ./scripts, ./web/**
  - Documentation, sample workflows, config templates, Docker deployment files, git hooks, and the TypeScript frontend (`web/agentd-ui`). These guide operators and provide the UI artifact served by agentd.

## 4. Entrypoints & startup flow
- **./cmd/agent/main.go**
  1. `config.Load()` → initialize observability (logger + OTEL) and HTTP client with extra headers.
  2. Optionally hydrate specialists from Postgres via `databases.NewSpecialistsStore`, falling back to YAML. When an `orchestrator` specialist exists it overrides OpenAI settings.
  3. Build the LLM provider via `llm/providers.Build`, register tool implementations (CLI exec, web search/fetch, patch, textsplitter, utility textbox, TTS, llm_transform, specialists, MCP tools, optional WARPP workflows) inside a `tools.Registry`.
  4. If `-warpp` is set, execute WARPP workflow detection/personalization/execution via `warpp.Runner` and exit.
  5. Otherwise, run `agent.Engine.Run` with the configured max steps and timeout, emitting all tool call callbacks to stdout.

- **./cmd/agentd/main.go → internal/agentd.Run()**
  1. Load `.env`/`config.yaml`, start logger/OTel, and construct the shared HTTP client and LLM provider (plus summary provider).
  2. Build a base tool registry (CLI, web search/fetch/screenshot, patch, textsplitter, textbox, TTS, Kafka, RAG ingest/retrieve, llm_transform, image describe, specialists, agent-call, ask_agent, multi_tool_use) and filter it per `enableTools` and allow lists.
  3. Initialize MCP manager and register configured (and DB-stored) servers, WARPP workflows (filesystem or DB), and specialized stores (chat, playground, projects, auth, specialists, MCP, WARPP) via `databases.Manager`.
  4. Construct `agent.Engine`, memory summarizer, playground service (prompts/datasets/experiments), projects service, optional whisper model for STT, auth provider, specialists cache overlay, token metrics reporter.
  5. Register HTTP routes (`router.go`) tying handlers to services (chat, tools, projects, config, specialists, WARPP, MCP OAuth, audio/STT, agent run/vision, metrics, auth endpoints) and wire the SPA frontend (with optional dev proxy + auth gate).
  6. Listen on `:32180`, serving both API and frontend.

- **./cmd/orchestrator/main.go**
  1. Load config/environment for Kafka brokers/topics, Redis dedupe, worker counts, timeouts.
  2. Initialize Redis-based dedupe store, Kafka producer, logging, OTEL, HTTP client, LLM provider, tool registry (CLI, web, patch, TTS, llm_transform, specialists, Kafka send tool), MCP manager, and WARPP runner (loaded from DB if available).
  3. Wrap WARPP runner with `internal/orchestrator.NewWarppAdapter` → exposes `Runner.Execute` to Kafka handler.
  4. Ensure Kafka topics exist, then start `orchestrator.StartKafkaConsumer`, which spins worker goroutines that:
     - Parse `CommandEnvelope`, dedupe by correlation ID, pick reply topic/DLQ.
     - Execute workflows with timeout using the WARPP adapter, streaming step results via publisher callbacks.
     - Publish `ResponseEnvelope` success/error payloads and persist dedupe results.

- **./cmd/whisper-go/main.go**
  - Minimal CLI taking `-model` and audio path, uses `github.com/ggerganov/whisper.cpp` bindings to load WAV files, convert samples, run inference, and print timestamps + transcript. Useful for testing the STT endpoint or offline batching.

## 5. Where to start reading
1. **`README.md` & `QUICKSTART.md`** – understand the product pitch, feature list, and basic deployment expectations.
2. **`internal/config/config.go`** – see every configuration knob; this clarifies how binaries are parameterized (LLM provider choice, tool toggles, MCP servers, DB backends, auth, etc.).
3. **`cmd/agent/main.go` + `internal/agent/engine.go`** – learn the core agent loop, how tool schemas are built, and how the engine coordinates LLM/tool interactions.
4. **`internal/tools/` registry + key tool packages** – understand the concrete affordances an agent has (CLI, web, RAG, specialists, parallel wrapper) and how to add new tools.
5. **`internal/agentd/run.go` & handler files** – see how the server composes the engine with HTTP APIs, memory summarization, playground, projects, STT, MCP OAuth, and authentication.
6. **`internal/orchestrator/handler.go` & `cmd/orchestrator/main.go`** – if you need Kafka-driven automation, study the message envelopes, dedupe logic, and WARPP adapter.
7. **`internal/warpp` + `docs/warpp.md` + `examples/workflows/*`** – to extend deterministic workflows, read how intents, guards, and tool references are structured.
8. **`internal/persistence/databases/factory.go` & RAG/playground packages** – explore how data storage backends are swapped and how higher-level services (playground experiments, RAG ingestion) piggyback on the same manager.

This progression moves from global context → configuration → core runtime → transport-specific layers, giving new contributors a mental model before diving into specialized subsystems (auth, MCP integration, front-end).
