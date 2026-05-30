# Runtime Flow: Code QA Run

CodeQA runs can be created through HTTP or agent tools. The service packages a diff, runs gates, asks judges, aggregates results, and writes artifacts/events.

## State Diagram

```mermaid
stateDiagram-v2
    [*] --> Requested
    Requested --> DiffPackaged
    DiffPackaged --> GatesRunning
    GatesRunning --> JudgesRunning
    JudgesRunning --> Aggregating
    Aggregating --> Accepted: aggregate action accept
    Aggregating --> HumanReview: aggregate action human_review
    Aggregating --> Rejected: aggregate action reject
    DiffPackaged --> Failed: diff/package error
    GatesRunning --> Failed: hard gate failure or command error
    JudgesRunning --> Failed: judge infrastructure error
    Accepted --> [*]
    HumanReview --> [*]
    Rejected --> [*]
    Failed --> [*]
```

## Sequence

```mermaid
sequenceDiagram
    participant Tool as Tool/API
    participant Runtime as agentd CodeQA runtime
    participant Service as codeqa.Service
    participant Store as CodeQA store
    participant Gates as Gates
    participant LLM as Judge LLM
    participant Artifacts as Artifact files

    Tool->>Runtime: RunRequest
    Runtime->>Service: Start run
    Service->>Store: Persist started event
    Service->>Gates: Execute deterministic checks
    Service->>LLM: Judge diff and gates
    Service->>Service: Aggregate verdicts
    Service->>Artifacts: Write report and JSON result
    Service->>Store: Persist completed result/events
    Runtime-->>Tool: RunResult or event stream
```

## Evidence

- `internal/codeqa/types.go`, `config.go`, `service/run.go`, `service/report.go`.
- `internal/codeqa/gates/*`, `internal/codeqa/judge/*`, `internal/codeqa/diff/*`.
- `internal/codeqa/store/postgres.go` creates `codeqa_runs` and `codeqa_run_events`.
- `internal/agentd/handlers_codeqa.go` exposes run and event endpoints.
- `internal/tools/codeqa/tool.go` exposes agent tools.
