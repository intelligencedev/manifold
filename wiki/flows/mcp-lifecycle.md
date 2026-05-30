# Runtime Flow: MCP Lifecycle

MCP servers can be configured in YAML or persisted through the API. They register tools into the Manifold tool registry.

## Server Registration Flow

```mermaid
sequenceDiagram
    participant Config as mcp.yaml / DB records
    participant Agentd as app_init and handlers
    participant Manager as mcpclient.Manager
    participant Registry as Tool registry
    participant MCP as MCP server

    Agentd->>Config: Load server configs
    Agentd->>Manager: RegisterFromConfig / RegisterOne
    Manager->>MCP: Start/connect and initialize
    MCP-->>Manager: List tools and schemas
    Manager->>Registry: Register tool wrappers
    Registry-->>Agentd: schemas visible to agents
```

## OAuth Flow

```mermaid
sequenceDiagram
    participant UI as MCP UI
    participant Agentd as MCP OAuth handler
    participant Metadata as Resource/Auth metadata
    participant Auth as Authorization server
    participant Store as MCP store

    UI->>Agentd: Start OAuth for server
    Agentd->>Metadata: Discover resource/auth metadata
    Agentd->>Auth: Register client if needed
    Agentd-->>UI: Authorization redirect/bootstrap URL
    UI->>Auth: User authorizes
    Auth-->>Agentd: callback code/state
    Agentd->>Auth: Exchange code for token
    Agentd->>Store: Persist server/token fields
    Agentd->>Agentd: Refresh/register MCP server
```

## Path-Dependent Server State

```mermaid
stateDiagram-v2
    [*] --> Configured
    Configured --> SharedRegistered: not path-dependent
    Configured --> WaitingForProject: path-dependent
    WaitingForProject --> UserSessionActive: user selects project
    UserSessionActive --> Replaced: user switches project
    Replaced --> UserSessionActive
    UserSessionActive --> Reaped: idle timeout
    SharedRegistered --> Closed: shutdown/delete
    Reaped --> WaitingForProject
    Closed --> [*]
```

## Evidence

- `mcp.yaml.example` documents server fields.
- `internal/mcpclient/mcpclient.go` registers configured servers.
- `internal/mcpclient/pool.go` manages path-dependent per-user sessions.
- `internal/agentd/handlers_mcp.go` implements CRUD and OAuth handlers.
- `docs/mcp.md` describes MCP configuration and usage.
