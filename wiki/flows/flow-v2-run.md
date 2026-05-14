# Runtime Flow: Flow v2 Run

Flow v2 run execution turns a saved workflow definition into ordered node events and outputs.

## Sequence

```mermaid
sequenceDiagram
    participant Client as UI/API client
    participant Handler as Flow handler
    participant Runtime as flowV2Runtime
    participant Store as Flow store
    participant Compiler as flow.Compiler
    participant Tools as Tool registry

    Client->>Handler: POST /api/flows/v2/run
    Handler->>Runtime: Run workflow request
    Runtime->>Store: Load workflow
    Runtime->>Compiler: ValidateWorkflow and Compile
    alt validation errors
        Runtime-->>Handler: rejected diagnostics
        Handler-->>Client: error response
    else valid
        Runtime->>Store: Create run/event state
        Runtime->>Tools: Execute ready nodes
        Tools-->>Runtime: Node outputs/errors
        Runtime->>Store: Append ordered events
        Handler-->>Client: run ID
        Client->>Handler: GET /api/flows/v2/runs/{run_id}/events
        Handler-->>Client: JSON or SSE events
    end
```

## Event Types to Preserve

The frontend depends on run and node lifecycle events. Preserve event names and ordering unless coordinating frontend changes.

## Validation Before Execution

The compiler should catch invalid workflow IDs, missing names, invalid trigger types, duplicate nodes, invalid node kinds, invalid retries/backoff, missing edge endpoints, and cycles.

## Evidence

- `internal/flow/types.go`, `internal/flow/compiler.go`.
- `internal/agentd/handlers_flow_v2.go`.
- `internal/agentd/flow_v2_runtime.go`.
- `internal/agentd/flow_v2_runtime_test.go` and `internal/flow/compiler_test.go`.
