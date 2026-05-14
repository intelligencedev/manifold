# Runtime Flow: Matrix and Pulse

This flow combines inbound Matrix messages and scheduled Pulse tasks.

## Matrix Message Flow

```mermaid
sequenceDiagram
    participant Matrix as Matrix homeserver
    participant Gateway as Matrix gateway
    participant Route as Room router
    participant Agentd as agentd matrix handler
    participant Chat as Chat runtime
    participant Store as Matrix/chat stores

    Matrix-->>Gateway: sync event
    Gateway->>Gateway: skip old/duplicate/self events
    Gateway->>Route: route message by mention/default
    Route-->>Gateway: target + prompt
    Gateway->>Agentd: HandleMatrixMessage
    Agentd->>Store: Persist inbound message and load room session
    Agentd->>Chat: Execute target with room/project context
    Chat-->>Agentd: assistant result/media
    Agentd->>Store: Persist outbound message
    Agentd-->>Matrix: post reply/media
```

## Pulse Task Flow

```mermaid
sequenceDiagram
    participant Runtime as pulseRuntime
    participant Store as Pulse store
    participant Service as pulse.Service
    participant Chat as Chat runtime
    participant Matrix as Matrix room

    Runtime->>Store: List rooms/tasks
    Runtime->>Service: Evaluate due tasks
    Service-->>Runtime: Plan with due tasks
    Runtime->>Store: Claim room lease
    Runtime->>Service: Build task prompt
    Runtime->>Chat: Run target specialist/team/orchestrator
    Chat-->>Runtime: result or error
    Runtime->>Store: Complete room pulse
    Chat-->>Matrix: optional room-facing message via matrix_room_message
```

## Evidence

- `internal/matrixgw/loop.go`, `router.go`, `service.go`, `poster.go`.
- `internal/agentd/matrix_gateway.go`, `matrix_projects.go`, `matrix_pulse.go`.
- `internal/pulse/service.go`.
- `internal/persistence/databases/pulse_store_postgres.go`, `matrix_message_store_postgres.go`.
