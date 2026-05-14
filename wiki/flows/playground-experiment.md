# Runtime Flow: Playground Experiment

The Playground lets developers compare prompt variants over datasets and evaluators.

## Sequence

```mermaid
sequenceDiagram
    participant UI as Playground UI
    participant API as Playground HTTP API
    participant Service as playground.Service
    participant Store as Store
    participant Planner as Planner
    participant Worker as Worker
    participant Evaluator as Evaluators

    UI->>API: Create prompt versions and dataset rows
    API->>Service: Persist prompt/dataset
    UI->>API: Create experiment with variants/evaluators
    Service->>Store: Save experiment
    UI->>API: Start run
    Service->>Store: Ensure no active run
    Service->>Planner: Plan shards from rows + variants
    Service->>Store: Create pending run
    loop tasks
        Service->>Worker: Execute prompt variant on row
        Worker-->>Service: output
    end
    Service->>Evaluator: Score outputs
    Evaluator-->>Service: aggregate and per-sample scores
    Service->>Store: Append results and complete run
    API-->>UI: run status, metrics, results
```

## Evidence

- `internal/playground/api.go` service orchestration.
- `internal/playground/experiment/spec.go` planner and experiment types.
- `internal/playground/worker/worker.go` task execution abstractions.
- `internal/playground/eval/eval.go` evaluator registry and runner.
- `internal/persistence/databases/playground_store.go` tables.
- `web/agentd-ui/src/views/playground/*` UI pages.
