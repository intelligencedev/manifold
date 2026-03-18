# Plan: True Branch Parallelism for Flow V2

## Problem Statement

The Flow V2 runtime executes all nodes sequentially via a flat `for _, nodeID := range plan.NodeOrder` loop, despite the compiler producing a valid DAG with dependency edges (`Incoming`/`Outgoing`). Independent branches (nodes with no data dependency between them) should execute concurrently.

**Example:** Given a DAG where node A fans out to B and C (both depend only on A), B and C currently run one after the other. They should run in parallel.

---

## Current Architecture

| Layer | File | Role |
|-------|------|------|
| Compiler | `internal/flow/compiler.go` | Kahn's toposort → `Plan{NodeOrder, Incoming, Outgoing}` |
| Types | `internal/flow/types.go` | `WorkflowSettings.MaxConcurrency` (defined, **unused**) |
| Runtime | `internal/agentd/flow_v2_runtime.go` | `executeFlowV2Run()` — serial loop over `plan.NodeOrder` |
| Events | `internal/flow/contracts.go` | `RunEvent` with per-node granularity |

The compiler already has everything needed: `Incoming[nodeID]` tells you exactly which predecessor nodes must complete before `nodeID` can start.

---

## Design

### Core Concept: Ready-Queue Scheduler

Replace the serial `for` loop with a **dependency-driven ready-queue** pattern:

1. Track a `remaining` indegree count per node.
2. Seed the queue with all nodes that have indegree 0 (root nodes).
3. When a node completes, decrement indegree of all its successors (`Outgoing`). Any successor reaching indegree 0 joins the ready queue.
4. Launch ready nodes concurrently, bounded by `MaxConcurrency`.
5. Run completes when all nodes have been processed (or a fatal error aborts).

This is essentially Kahn's algorithm executed at **runtime** instead of at compile time, with the addition of a worker pool.

---

## Implementation Phases

### Phase 1 — Compiler: Add `Indegree` Map to Plan

**File:** `internal/flow/compiler.go`, `internal/flow/types.go`

Add an `Indegree map[string]int` field to the `Plan` struct so the runtime doesn't need to recompute it:

```go
// types.go
type Plan struct {
    WorkflowID string            `json:"workflow_id"`
    NodeOrder  []string          `json:"node_order"`    // keep for deterministic fallback / logging
    Incoming   map[string][]Edge `json:"incoming"`
    Outgoing   map[string][]Edge `json:"outgoing"`
    Indegree   map[string]int    `json:"indegree"`      // NEW
}
```

```go
// compiler.go — at the end of CompileWorkflow, before returning:
indegreeSnap := make(map[string]int, len(wf.Nodes))
for _, n := range wf.Nodes {
    indegreeSnap[n.ID] = 0
}
for _, e := range wf.Edges {
    indegreeSnap[e.Target.NodeID]++
}
plan.Indegree = indegreeSnap
```

**Tests:** Update `compiler_test.go` — assert `plan.Indegree` values for existing test workflows.

---

### Phase 2 — Runtime: Replace Serial Loop with Parallel Scheduler

**File:** `internal/agentd/flow_v2_runtime.go`

Replace the body of `executeFlowV2Run` (the `for _, nodeID := range plan.NodeOrder` loop) with a concurrent scheduler.

#### 2a. Data structures

```go
type nodeResult struct {
    nodeID string
    output map[string]any
    err    error
    skip   bool   // true when guard evaluated to false
}
```

#### 2b. Scheduler skeleton

```go
func (a *app) executeFlowV2Run(ctx context.Context, userID int64, runID string,
    wf flow.Workflow, plan *flow.Plan, input map[string]any) {

    emit := func(ev flow.RunEvent) {
        _ = a.flowV2State().appendRunEvent(userID, runID, ev)
    }
    emit(flow.RunEvent{Type: flow.RunEventTypeRunStarted, Status: "running", Message: "run started"})

    nodeByID := make(map[string]flow.Node, len(wf.Nodes))
    for _, n := range wf.Nodes { nodeByID[n.ID] = n }

    reg := a.flowV2ExecutionRegistry()
    toolSet := map[string]bool{}
    if reg != nil {
        for _, s := range reg.Schemas() { toolSet[s.Name] = true }
    }
    defaultExec := wf.Settings.DefaultExecution

    // --- concurrency setup ---
    maxC := wf.Settings.MaxConcurrency
    if maxC <= 0 { maxC = 4 }                      // sensible default
    sem := make(chan struct{}, maxC)                 // semaphore

    // mutable state protected by mu
    var mu       sync.Mutex
    remaining := make(map[string]int, len(plan.Indegree))
    for id, d := range plan.Indegree { remaining[id] = d }
    nodeOutputs := make(map[string]map[string]any, len(wf.Nodes))
    skipped     := map[string]bool{}

    resultCh := make(chan nodeResult, len(wf.Nodes))
    var wg     sync.WaitGroup
    runCtx, cancelRun := context.WithCancel(ctx)
    defer cancelRun()
    var runFailed bool

    // launch launches a single node in its own goroutine (bounded by sem).
    launch := func(nodeID string) {
        wg.Add(1)
        go func() {
            defer wg.Done()
            sem <- struct{}{}        // acquire
            defer func() { <-sem }() // release

            node := nodeByID[nodeID]
            emit(flow.RunEvent{Type: flow.RunEventTypeNodeStarted, NodeID: nodeID,
                Status: "running", Message: "node started"})

            // Guard evaluation — skip node if guard is false
            // (see Phase 4 for details)

            mu.Lock()
            resolvedInputs, err := resolveNodeInputs(node, plan.Incoming[nodeID], nodeOutputs, input)
            mu.Unlock()
            if err != nil {
                resultCh <- nodeResult{nodeID: nodeID, err: err}
                return
            }

            output, err := a.executeFlowV2NodeWithRetries(runCtx, node, resolvedInputs,
                reg, toolSet, defaultExec, emit)
            resultCh <- nodeResult{nodeID: nodeID, output: output, err: err}
        }()
    }

    // Seed: launch all root nodes (indegree 0)
    for _, n := range wf.Nodes {
        if remaining[n.ID] == 0 {
            launch(n.ID)
        }
    }

    // Drain: collect results, propagate, launch newly-ready nodes
    completed := 0
    total := len(wf.Nodes)
    for completed < total {
        res := <-resultCh
        completed++

        if res.err != nil {
            emit(flow.RunEvent{Type: flow.RunEventTypeNodeFailed, NodeID: res.nodeID,
                Status: "failed", Error: res.err.Error(), Message: "node failed"})
            if effectiveOnError(nodeByID[res.nodeID], defaultExec) != flow.ErrorStrategyContinue {
                runFailed = true
                cancelRun()
                // drain remaining in-flight
                for completed < total {
                    <-resultCh; completed++
                }
                emit(flow.RunEvent{Type: flow.RunEventTypeRunFailed, Status: "failed",
                    Error: res.err.Error(), Message: "run failed"})
                return
            }
            // on_error=continue → mark as done, propagate skip to dependents
            mu.Lock()
            skipped[res.nodeID] = true
            mu.Unlock()
        } else if !res.skip {
            mu.Lock()
            nodeOutputs[res.nodeID] = res.output
            mu.Unlock()
            emit(flow.RunEvent{Type: flow.RunEventTypeNodeCompleted, NodeID: res.nodeID,
                Status: "completed", Output: cloneMap(res.output), Message: "node completed"})
        }

        // Propagate: decrement successors, launch newly-ready
        mu.Lock()
        for _, edge := range plan.Outgoing[res.nodeID] {
            remaining[edge.Target.NodeID]--
            if remaining[edge.Target.NodeID] == 0 {
                launch(edge.Target.NodeID)
            }
        }
        mu.Unlock()
    }
    wg.Wait()

    if runCtx.Err() != nil && !runFailed {
        emit(flow.RunEvent{Type: flow.RunEventTypeRunFailed, Status: "failed",
            Error: runCtx.Err().Error(), Message: "run cancelled"})
        return
    }
    emit(flow.RunEvent{Type: flow.RunEventTypeRunCompleted, Status: "completed", Message: "run completed"})
}
```

Key properties:
- **Bounded concurrency** via `sem` channel of size `MaxConcurrency`.
- **Thread-safe `nodeOutputs`** — locked only for the brief read during `resolveNodeInputs` and write on completion.
- **Deterministic root ordering** preserved by iterating `wf.Nodes` (declaration order) to seed the queue.
- **Fail-fast** — when a node fails with `on_error=fail`, the run context is cancelled and all in-flight goroutines see the cancellation.

---

### Phase 3 — Extract Retry Logic into Helper

**File:** `internal/agentd/flow_v2_runtime.go`

The current retry loop is inline in the serial `for` loop. Extract it so it can be called from the goroutine cleanly:

```go
func (a *app) executeFlowV2NodeWithRetries(
    ctx context.Context,
    node flow.Node,
    inputs map[string]any,
    reg tools.Registry,
    toolSet map[string]bool,
    defaults flow.NodeExecution,
    emit func(flow.RunEvent),
) (map[string]any, error) {
    attempts := effectiveRetries(node, defaults)
    var output map[string]any
    var err error
    for attempt := 1; attempt <= attempts; attempt++ {
        output, err = a.executeFlowV2Node(ctx, node, inputs, reg, toolSet, defaults)
        if err == nil {
            return output, nil
        }
        if attempt < attempts {
            emit(flow.RunEvent{
                Type:    flow.RunEventTypeNodeRetrying,
                NodeID:  node.ID,
                Status:  "retrying",
                Message: fmt.Sprintf("retry %d/%d", attempt, attempts-1),
                Error:   err.Error(),
            })
            if !sleepFlowRetry(ctx, node, defaults, attempt) {
                return nil, context.Canceled
            }
        }
    }
    return nil, err
}
```

---

### Phase 4 — Guard & Conditional Skip Propagation

**File:** `internal/agentd/flow_v2_runtime.go`

When a node has a `Guard` expression that evaluates to false, the node should be **skipped** and all downstream-only dependents should also be skipped (cascade).

Inside the `launch` goroutine, before input resolution:

```go
if guard := strings.TrimSpace(node.Guard); guard != "" {
    mu.Lock()
    v, err := evalFlowExpression(guard, input, nodeOutputs)
    mu.Unlock()
    if err == nil {
        if b, ok := asBool(v); ok && !b {
            emit(flow.RunEvent{Type: flow.RunEventTypeNodeSkipped, NodeID: nodeID,
                Status: "skipped", Message: "guard evaluated to false"})
            resultCh <- nodeResult{nodeID: nodeID, skip: true}
            return
        }
    }
}
```

For `if`-type nodes: when the condition is false, optionally mark the "false-branch" successors as skipped. This requires edge metadata (true/false port labels), which the existing `PortRef.Port` field can carry.

**Cascade logic:** In the propagation section, when a node is skipped and a successor **only** depends on skipped/failed nodes (all predecessors are in `skipped` map), auto-skip that successor too.

---

### Phase 5 — Event Sequence Ordering

**File:** `internal/agentd/flow_v2_runtime.go`

Events must have strictly monotonic `Sequence` numbers even when nodes run concurrently. The existing `appendRunEvent` already increments `Sequence` under a mutex, so this is already safe. Verify:

```go
func (s *flowV2Runtime) appendRunEvent(userID int64, runID string, ev flow.RunEvent) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    // ... increments run.Sequence atomically
}
```

No changes needed here — just confirm in a test that events from parallel nodes interleave with valid monotonic sequences.

---

### Phase 6 — Honour `MaxConcurrency` from Settings

**File:** `internal/agentd/flow_v2_runtime.go`

Already handled in Phase 2 via:
```go
maxC := wf.Settings.MaxConcurrency
if maxC <= 0 { maxC = 4 }
```

Add a `max_concurrency` field to the frontend workflow settings panel so users can tune it per workflow.

**Frontend file:** `web/agentd-ui/src/views/FlowView.vue` or workflow settings component — add a numeric input bound to `workflow.settings.max_concurrency`.

---

### Phase 7 — Frontend: Multiple Active Nodes

**File:** `web/agentd-ui/src/views/FlowView.vue`, `web/agentd-ui/src/stores/flowRun.ts`

The animated-edge feature already tracks `activeNodeIds: string[]` derived from `node_started` / `node_completed` events. It already supports multiple simultaneous active nodes. **No changes required** — this was designed for parallelism from the start.

Verify by testing: when two `node_started` events arrive before any `node_completed`, both nodes' outgoing edges should animate.

---

## Testing Strategy

### Unit Tests

| Test | File | What it verifies |
|------|------|-----------------|
| `TestPlanIndegree` | `internal/flow/compiler_test.go` | Indegree map matches expected values |
| `TestParallelFanOut` | `internal/agentd/flow_v2_runtime_test.go` | Two independent branches run concurrently (verify via timestamps or execution order) |
| `TestParallelConverge` | same | Convergence node waits for all predecessors |
| `TestMaxConcurrency` | same | With `MaxConcurrency=1`, execution is effectively serial |
| `TestFailFastCancellation` | same | On `on_error=fail`, in-flight sibling nodes see context cancellation |
| `TestErrorContinueParallel` | same | On `on_error=continue`, sibling branches keep running |
| `TestGuardSkipCascade` | same | Skipped node cascades skip to downstream-only nodes |
| `TestEventSequenceMonotonic` | same | Event sequence numbers are strictly increasing even with concurrency |
| `TestDiamondDAG` | same | A→{B,C}→D: B and C run in parallel, D waits for both |
| `TestSingleNode` | same | Degenerate case: single node still works |
| `TestAllRoots` | same | Disconnected graph: all nodes are roots, all run in parallel |

### Integration / E2E

- Build a diamond-shaped workflow in the UI, run it, observe that B and C show animated edges simultaneously.
- Verify the run trace shows overlapping `node_started` timestamps for independent siblings.

---

## Task Breakdown

| # | Task | Est. Scope | Dependencies |
|---|------|-----------|-------------|
| 1 | Add `Indegree` to `Plan` struct + compiler | S | — |
| 2 | Update compiler tests | S | 1 |
| 3 | Extract `executeFlowV2NodeWithRetries` helper | S | — |
| 4 | Implement parallel scheduler in `executeFlowV2Run` | M | 1, 3 |
| 5 | Implement guard skip + cascade propagation | S | 4 |
| 6 | Add parallel execution unit tests (fan-out, converge, diamond, fail-fast) | M | 4 |
| 7 | Verify event sequence monotonicity under concurrency | S | 4 |
| 8 | Add `max_concurrency` to frontend settings panel | S | — |
| 9 | E2E validation with diamond workflow | S | 4, 8 |

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Race on `nodeOutputs` map | Data corruption | Mutex around reads/writes; `resolveNodeInputs` takes lock for entire call |
| Non-deterministic execution order | Flaky tests | Use timestamps/counters for ordering assertions, not string ordering |
| Guard expressions reading not-yet-written outputs | Stale data | Guard eval runs under lock after predecessors complete (guaranteed by indegree gate) |
| `on_error=continue` with skipped predecessors | Downstream node gets nil inputs | `resolveNodeInputs` already handles `nil` source gracefully; add explicit skip cascade |
| `MaxConcurrency=1` should behave like current serial mode | Regression | Dedicated test case |

---

## Migration / Backward Compatibility

- The `Plan.NodeOrder` field is **preserved** for logging, serialization, and as a fallback reference. Existing persisted plans remain valid.
- Workflows with no parallel branches naturally serialize (each node has indegree 1, only one node is ever "ready").
- `MaxConcurrency` defaults to 4 if unset — existing workflows that omit it gain parallelism automatically. Set to 1 to opt out.
- Event contract is unchanged — only the temporal interleaving of events changes.
