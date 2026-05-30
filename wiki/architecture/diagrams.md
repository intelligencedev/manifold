# Diagram Catalog

This page gathers the most important Mermaid diagrams for onboarding. Each diagram uses the diagram type that best matches the information being shown.

## Component Map (`flowchart`)

Use this to understand where code changes usually land.

```mermaid
flowchart LR
    Frontend[Vue frontend] -->|axios/fetch| API[agentd HTTP API]
    API --> Chat[Chat handlers]
    API --> Projects[Project handlers]
    API --> Flow[Flow v2 handlers]
    API --> MCP[MCP handlers]
    API --> Metrics[Metrics/debug handlers]
    API --> Playground[Playground handler]

    Chat --> Agent[Agent Engine]
    Agent --> LLM[LLM providers]
    Agent --> ToolRegistry[Tool Registry]
    Agent --> Memory[Memory managers]
    ToolRegistry --> BuiltinTools[Built-in tools]
    ToolRegistry --> MCPTools[MCP tools]
    Projects --> Workdir[Configured workdir]
    Flow --> ToolRegistry
    Playground --> PlaygroundStore[(Playground store)]
    Memory --> Stores[(Search/vector/graph/chat stores)]
    API --> Stores
```

**Evidence:** `internal/agentd/router.go`, `internal/agentd/app_init.go`, `internal/agent/engine*.go`, `web/agentd-ui/src/api/*`.

## Chat Request (`sequenceDiagram`)

Use this when changing chat handlers, streaming, or agent execution.

```mermaid
sequenceDiagram
    participant UI as UI or API client
    participant H as agentd chat handler
    participant S as Chat store
    participant W as Workspace manager
    participant E as Agent engine
    participant L as LLM provider
    participant T as Tool registry

    UI->>H: POST /api/prompt or /agent/run
    H->>S: Load/create session and history
    H->>W: Resolve project workspace if project_id set
    H->>E: Build engine for target specialist/team/orchestrator
    E->>L: Send system, history, current request
    L-->>E: Text delta or tool call
    E->>T: Dispatch approved tool calls
    T-->>E: Tool results
    E->>L: Continue with tool results
    L-->>E: Final assistant response
    E-->>H: Final text and trace events
    H->>S: Persist messages, summary, activities
    H-->>UI: JSON or server-sent events
```

**Evidence:** `internal/agentd/handlers_chat.go`, `internal/agentd/chat_handler_setup.go`, `internal/agentd/chat_execution.go`, `internal/agent/engine_loop.go`, `internal/agent/engine_tools.go`.

## Workflow Lifecycle (`stateDiagram-v2`)

Use this for Flow v2 run state and event behavior.

```mermaid
stateDiagram-v2
    [*] --> Submitted
    Submitted --> Validating
    Validating --> Rejected: validation diagnostics contain errors
    Validating --> Queued: workflow compiles
    Queued --> Running
    Running --> NodeStarted
    NodeStarted --> NodeCompleted: tool/action succeeds
    NodeStarted --> NodeRetried: retry policy allows retry
    NodeRetried --> NodeStarted
    NodeStarted --> NodeFailed: error strategy fail
    NodeStarted --> NodeSkipped: guard false or cascade skip
    NodeCompleted --> Running: more nodes ready
    NodeSkipped --> Running: downstream can continue
    NodeFailed --> Failed
    Running --> Completed: all required nodes done
    Completed --> [*]
    Failed --> [*]
    Rejected --> [*]
```

**Evidence:** `internal/flow/types.go`, `internal/flow/compiler.go`, `internal/agentd/flow_v2_runtime.go`, `docs/parallel-execution-plan.md`.

## Persistence Overview (`erDiagram`)

Use this for database-impacting changes.

```mermaid
erDiagram
    USERS ||--o{ SESSIONS : owns
    USERS ||--o{ USER_ROLES : has
    ROLES ||--o{ USER_ROLES : grants
    CHAT_SESSIONS ||--o{ CHAT_MESSAGES : contains
    CHAT_SESSIONS ||--o{ CHAT_AGENT_ACTIVITIES : traces
    PROJECTS ||--o{ PROJECT_FILES : indexes
    SPECIALISTS ||--o{ SPECIALIST_GROUP_MEMBERSHIPS : member
    SPECIALIST_GROUPS ||--o{ SPECIALIST_GROUP_MEMBERSHIPS : contains
    FLOW_V2_WORKFLOWS ||--o{ WARPP_TOOLS : exposed_as
    PULSE_ROOMS ||--o{ PULSE_TASKS : schedules
    BELIEF_SCOPES ||--o{ BELIEF_EPISODES : contains
    BELIEF_SCOPES ||--o{ BELIEFS : contains
    BELIEFS ||--o{ BELIEF_EVIDENCE : supported_by
    BELIEFS ||--o{ BELIEF_PROMOTIONS : promoted_by
    PLAYGROUND_EXPERIMENTS ||--o{ PLAYGROUND_RUNS : starts
    PLAYGROUND_RUNS ||--o{ PLAYGROUND_RUN_RESULTS : records
```

**Evidence:** SQL initialization in `internal/auth/store.go`, `internal/persistence/databases/*.go`, `internal/codeqa/store/postgres.go`.

## Tool Model (`classDiagram`)

Use this when changing built-in tools or MCP registration.

```mermaid
classDiagram
    class Tool {
      +Name() string
      +JSONSchema() map
      +Call(ctx, raw) any,error
    }
    class Registry {
      +Register(Tool)
      +Unregister(name)
      +Schemas() []ToolSchema
      +Dispatch(ctx,name,args) []byte,error
    }
    class DiscoverableRegistry {
      +Search/load tools from ToolIndex
    }
    class MCPManager {
      +RegisterFromConfig(ctx, registry, config)
      +RegisterOne(ctx, registry, server)
      +Close()
    }
    class AgentEngine {
      +Tools Registry
    }
    Tool <|.. BuiltInTool
    Tool <|.. MCPTool
    Registry o-- Tool
    DiscoverableRegistry --|> Registry
    MCPManager --> Registry
    AgentEngine --> Registry
```

**Evidence:** `internal/tools/types.go`, `internal/tools/registry*.go`, `internal/tools/discovery/*`, `internal/mcpclient/*`, `internal/agent/engine.go`.
