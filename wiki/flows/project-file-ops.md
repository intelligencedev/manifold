# Runtime Flow: Project File Operations

Project file operations appear through HTTP project APIs and agent file tools. Both must preserve workspace containment.

## HTTP Project Operations

```mermaid
sequenceDiagram
    participant UI as Projects UI
    participant Handler as Project handler
    participant Service as projects.Service
    participant FS as Project filesystem
    participant Meta as .meta/project.json

    UI->>Handler: List/create/read/write/delete/move/upload/archive
    Handler->>Handler: Resolve authenticated or system user
    Handler->>Service: Validate project and relative path
    Service->>FS: Perform filesystem operation under project root
    Service->>Meta: Update generation/updatedAt if needed
    Handler-->>UI: JSON, file bytes, or archive stream
```

## Agent File Tool Operations

```mermaid
sequenceDiagram
    participant Agent
    participant Tool as file_* tool
    participant Guard as rootGuard/path resolver
    participant FS as Current project workspace

    Agent->>Tool: JSON args with relative path
    Tool->>Guard: Resolve base dir from context and allowed roots
    Guard-->>Tool: safe relative and full path
    Tool->>FS: Read/write/patch/delete
    Tool-->>Agent: structured result
```

## Safety Invariants

- Project IDs must validate.
- Paths must resolve under project root/current workspace root.
- Symlinks require careful handling; file tools reject symlink operations in dangerous paths.
- Recursive deletion requires explicit intent.
- Archive and upload paths must not escape the project.

## Evidence

- `internal/projects/service.go` implements filesystem project operations.
- `internal/agentd/handlers_projects.go` exposes HTTP operations.
- `internal/workspaces/manager.go` resolves workspaces.
- `internal/sandbox/pathpolicy.go` and `internal/tools/filetool/tool.go` enforce path constraints.
