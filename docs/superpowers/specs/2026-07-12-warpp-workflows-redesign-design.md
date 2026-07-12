# WARPP Workflows Redesign — Typed-Port Dataflow

**Date:** 2026-07-12
**Status:** Approved
**Replaces:** Flow v2 (`internal/flow`, `internal/agentd/flow_v2_*`), legacy WARPP runner format (`docs/warpp.md`), `warpptool`, `FlowView.vue` + `flowEditorCompat.ts`

## 1. Problem

The current workflows feature is hard to use. Root causes, confirmed in code:

- **Three overlapping data-passing mechanisms** live at once: edge-level field
  mappings (`Edge.Mapping`), per-node input bindings with expression strings
  (`={{$node.X.output.Y}}`, `={{$run.input.query}}`), and legacy `${A.*}`
  attribute templating kept alive by a frontend compat layer
  (`flowEditorCompat.ts`) that round-trips every workflow between two formats.
- **Node outputs have no defined shape.** A tool node emits
  `{inputs, payload: "<json string>", json: <parsed>, ...parsed keys flattened}`.
  The same value is reachable at three paths; the path resolver JSON-parses
  strings mid-traversal to cope; `unwrapWorkflowPayload` guesses where the real
  output lives. Authors cannot know what `$node.x.output.???` should be without
  running the workflow.
- **Accreted concepts:** `kind` vs `type` on nodes, `guard` strings,
  `publish_result`/`publish_mode`, trigger machinery, and a 3,559-line
  `FlowView.vue` monolith.
- **No fan-out.** Despite the DAG ambition, there is no map/iterate construct.

## 2. Decisions from requirements review

| Question | Decision |
|---|---|
| Primary authors | Humans on a canvas **and** the LLM, equally. One canonical format, no compat layers. |
| Workflow complexity | Full DAG: parallel branches, fan-out over lists (subgraph per item), joins. |
| Migration | **Clean break.** Existing workflows are dropped; no importer, no ongoing compat. |
| Integrations kept | Workflows exposed as agent tools; durable/resumable runs; live SSE run view. |
| Integrations dropped | Schedule/webhook/event triggers. Runs start manually (UI/API) or via agent tool call. |

## 3. Core principle

**A node is a pure function with declared, typed ports. Data passing is the
wire — nothing else.** (ComfyUI's model.) There are no expressions, no shared
attribute map, no edge transforms. Reshaping data is done by small, visible
data nodes. Control flow is gated wires, not guard strings.

## 4. The workflow document

One JSON document. Wiring lives inside each input binding; there is no
separate edges array (the canvas derives edges from `from` refs).

```json
{
  "id": "research-brief",
  "name": "Research brief",
  "description": "Search the web and summarize findings.",
  "inputs": [
    { "name": "topic", "type": "text", "required": true }
  ],
  "nodes": [
    {
      "id": "search",
      "type": "tool.web_search",
      "inputs": {
        "query":       { "from": "in.topic" },
        "max_results": { "value": 5 }
      }
    },
    {
      "id": "summarize",
      "type": "tool.llm_transform",
      "inputs": {
        "instruction": { "value": "Summarize these results" },
        "input":       { "from": "search.results_text" }
      }
    }
  ],
  "outputs": { "brief": { "from": "summarize.text" } },
  "settings": {
    "max_concurrency": 4,
    "default_policy": { "timeout": "60s", "retries": { "max": 0, "backoff": "fixed" }, "on_error": "fail" }
  },
  "publish": { "tool": true }
}
```

Rules:

- An input binding is **exactly one of** `{"from": "nodeID.port"}` or
  `{"value": <JSON literal>}`. Expression strings do not exist.
- `in` is a virtual node exposing the workflow's own typed inputs.
- Node IDs are unique within their graph scope. Port addresses are
  `nodeID.portName`.
- Workflow `outputs` bind names to node output ports; this is the run result.
- `publish.tool: true` exposes the workflow as an agent tool (see §9).
- Canvas layout (positions, groups, sticky notes, sizes) stays in the existing
  canvas **sidecar**, keyed by node ID. It never affects execution.

## 5. Type system

Port types: `text`, `number`, `boolean`, `json`, `file`, `list<T>` where
`T ∈ {text, number, boolean, json, file}`.

- `file` is a path string with project-sandbox semantics (same rules as
  today's `workflowToolContext`).
- **Coercion table (fixed, complete):** `number → text`, `boolean → text`.
  Nothing else coerces implicitly. `json → text` requires `data.stringify`;
  `text → json` requires `data.parse`.
- **Type variables:** built-in logic/data nodes may declare ports typed `T`
  (single type variable per node, optionally `list<T>`). The validator unifies
  `T` across the node instance from connected wires/literals; if `T` cannot be
  resolved to one concrete type, that is a diagnostic.
- **Dynamic output (Extract/Constant only):** `data.extract` and
  `data.constant` declare an `as` config enum (`text|number|boolean|json|list<json>`,
  default `json`); their output port type resolves from the `as` literal at
  validation time. The runtime enforces the assertion and fails the node if the
  actual value does not conform.

Runtime values are JSON values tagged with their port type
(`warpp.Value{Type, Data}`). Adapters and data nodes validate values against
port types at node boundaries.

## 6. Node manifests

Every node type declares its interface once:

```go
type Manifest struct {
    Type        string     // "tool.web_search", "data.extract", "control.map"
    Title       string
    Category    string     // "tool" | "data" | "logic" | "control" | "flow"
    Description string
    Inputs      []PortSpec
    Outputs     []PortSpec
}

type PortSpec struct {
    Name        string
    Type        string  // port type, "T", "list<T>", or "dynamic:as"
    Required    bool
    Default     any     // literal default; implies not Required
    Variadic    string  // "" | "list" | "named"
    Description string
}
```

- An **unwired input port with a literal is the inspector widget** — widget and
  port are one mechanism (ComfyUI duality). All ports are wireable.
- **Variadic ports:** `list` variadic accepts an array of bindings
  (`"values": [{"from":"a.x"}, {"from":"b.y"}]`); `named` variadic accepts a
  map of bindings (`"vars": {"title": {"from":"x.y"}}`). At most one variadic
  input per node, declared last.
- `GET /api/warpp/catalog` serves all manifests **plus the coercion table** so
  the editor and the LLM consume the identical contract. The frontend
  type-compat check is a small pure function driven by that payload — no
  hardcoded duplicate rules.

### Tool adapters

A curated tool node is a thin adapter: build the tool's args from input port
values, dispatch via the existing `tools.Registry`, map the tool's raw JSON
result onto **declared output ports**. If the result does not match the
declared outputs, the node fails with a contract-violation error naming the
missing/mismatched field. The adapter is the only place that knows a tool's
raw shape.

**Initial curated set (10):** `tool.web_search`, `tool.web_fetch`,
`tool.llm_transform`, `tool.file_read`, `tool.file_write`,
`tool.rag_retrieve`, `tool.rag_ingest`, `tool.run_cli`, `tool.agent_call`,
`tool.matrix_room_message`.

**Escape hatch:** `tool.generic` — inputs: `tool: text` (literal-typical),
`args: json`; output: `result: json`. Covers the registry long tail; combine
with `data.extract` to reshape.

### Data nodes

| Type | Inputs | Outputs | Notes |
|---|---|---|---|
| `data.extract` | `source: json`, `path: text`, `as` config | `value: dynamic:as` | Dot/index path, e.g. `results.0.title`. No mid-path string parsing: values are structured already. |
| `data.template` | `template: text`, `vars: named-variadic T` | `text: text` | `{name}` slots; scalars render directly, `json` renders compact-stringified. |
| `data.merge` | `objects: list-variadic json` | `json: json` | Shallow merge, later wins. |
| `data.stringify` | `value: T` | `text: text` | Pretty-printed for `json`. |
| `data.parse` | `text: text` | `json: json` | Fails node on invalid JSON. |
| `data.constant` | `value` config, `as` config | `value: dynamic:as` | Shared literal feeding multiple nodes. |

### Logic nodes

| Type | Inputs | Outputs | Notes |
|---|---|---|---|
| `logic.if` | `condition: boolean`, `value: T` | `then: T`, `else: T` | Fires **exactly one** output per run. |
| `logic.coalesce` | `values: list-variadic T` | `value: T` | Emits the first input that fired; rejoins branches. |
| `logic.equals` | `a: T`, `b: T` | `result: boolean` | Deep equality for `json`. |
| `logic.contains` | `haystack: text`, `needle: text` | `result: boolean` | |
| `logic.not` | `value: boolean` | `result: boolean` | |
| `logic.greater_than` | `a: number`, `b: number` | `result: boolean` | |

### Control: `control.map`

```json
{
  "id": "per_result",
  "type": "control.map",
  "inputs": {
    "items":         { "from": "search.results" },
    "concurrency":   { "value": 4 },
    "on_item_error": { "value": "skip" }
  },
  "body": {
    "nodes": [
      { "id": "fetch", "type": "tool.web_fetch",
        "inputs": { "url": { "from": "item.value" } } }
    ],
    "outputs": { "result": { "from": "fetch.text" } }
  }
}
```

- `items: list<T>`; the body sees virtual node `item` with `value: T` and
  `index: number`.
- **Lexical scoping:** body nodes may also wire from any port visible to the
  Map node itself (outer nodes, `in`). Outer refs are resolved once and
  broadcast to all iterations; the validator treats each outer ref as a
  dependency edge of the Map node (cycle-checked).
- Body declares one `outputs` map; Map's output is `results: list<U>` with `U`
  unified from the body output. Skipped items (under `on_item_error: "skip"`)
  are omitted; surviving results keep item order.
- Iterations run concurrently up to `concurrency`. `on_item_error: "fail"`
  fails the run on first item failure.
- Maps nest.

## 7. Execution semantics

- A node runs **at most once** per run (per Map iteration): gather all inputs
  at start, execute, emit typed values on output ports at completion. No
  shared mutable context exists.
- **Readiness:** a node is ready when every wired input's source port has
  fired or been resolved as skipped.
- **Skip rule (complete):** if a *required* input's source is skipped or never
  fires (e.g., the unfired branch of `logic.if`), the node **skips**, and skips
  cascade. If an *optional* input's source is skipped, the node runs with the
  port's default.
- **Node policy:** `timeout` (Go duration string), `retries {max, backoff:
  fixed|exponential}`, `on_error: fail|skip`. `fail` fails the run; `skip`
  marks the node skipped (cascades) and the run continues. Workflow
  `default_policy`, per-node override.
- **Scheduling:** topological wavefront, global `max_concurrency` (default 4),
  deterministic tie-break by node declaration order.
- **Run statuses:** `running`, `completed`, `completed_with_skips`, `failed`,
  `cancelled`.

## 8. Engine, events, durability

New package **`internal/warpp`**: document types, manifests + catalog,
validator, scheduler/executor, value types. The proven bones of the current
executor carry over conceptually (wavefront scheduling, per-node
`durable.Step` checkpoints, retry/backoff, SSE fan-out) but are rewritten
around typed port values instead of `map[string]any` blobs.

- **Durable checkpoints:** step key is the node path; Map iterations
  checkpoint as `map_id[index]:node_id`. Resume replays completed checkpoints
  exactly as today's `durable.Step` does.
- **Events (SSE + JSON snapshot):** `run_started|run_completed|run_failed|run_cancelled`,
  `node_started|node_completed|node_failed|node_skipped|node_retrying`. Every
  run event carries the run `status` field; `run_completed` distinguishes
  `completed` from `completed_with_skips` there. Every
  node event carries `node_path` (includes Map iteration indices, e.g.
  `per_result[3].fetch`). `node_completed` carries per-port typed outputs so
  the run view can paint values onto wires live. Map emits its own
  node events plus child events for body nodes.
- **Validation** (`ValidateDocument`) returns diagnostics with severity, code,
  message, and a JSON-path — statically checks: unique IDs, known node types,
  binding exclusivity (`from` XOR `value`), all required ports satisfied,
  `from` refs resolve to real ports, type compatibility (with coercion table
  and type-variable unification), acyclicity (including Map outer-ref edges),
  Map body well-formedness, literal values conform to port types.

## 9. Workflows are nodes (composability)

A workflow's typed `inputs`/`outputs` are a manifest. This yields:

- **Agent tools:** for workflows with `publish.tool: true`, register an agent
  tool whose JSON schema is derived from the input ports; the tool result is
  exactly the declared `outputs` (e.g. `{"brief": "..."}`) plus `run_id` and
  `status`. Replaces `syncWarppTools`/`unwrapWorkflowPayload` guessing. Sync
  on save/delete as today.
- **Subflows:** a saved workflow can be placed as a node
  (`type: "flow.<workflow-id>"`) inside another workflow. The catalog includes
  published workflows. Validation rejects self/recursive inclusion cycles.
- **One run contract** regardless of caller (UI, agent tool, parent flow):
  typed values in on input ports → typed values out on output ports.

### Agent authoring tools

The agent gets these tools (replacing `warpptool`): `workflow_catalog`
(manifests + coercions), `workflow_list`, `workflow_get`, `workflow_save`
(validate + save; returns diagnostics on failure so the LLM can self-correct),
`workflow_run` (synchronous execute, returns declared outputs).

## 10. API surface

| Method & path | Purpose |
|---|---|
| `GET /api/warpp/workflows` | List summaries |
| `GET /api/warpp/workflows/{id}` | Document + canvas sidecar |
| `PUT /api/warpp/workflows/{id}` | Validate + upsert (400 with diagnostics on error) |
| `DELETE /api/warpp/workflows/{id}` | Delete (unregisters tool if published) |
| `POST /api/warpp/validate` | Diagnostics for a document without saving |
| `POST /api/warpp/runs` | Start run `{workflow_id, input}` → `{run_id}` (202) |
| `GET /api/warpp/runs/{id}/events` | SSE stream or JSON snapshot (Accept-negotiated) |
| `GET /api/warpp/catalog` | Manifests + coercion table + published workflows |

Auth/user scoping matches today's flow v2 handlers. Storage: new
`warpp_workflows` table (document + canvas sidecar columns) via the existing
persistence manager; runs/events remain in-memory with durable event recording
as today. A migration drops the flow v2 tables (clean break).

## 11. Editor (Vue Flow stays; the model changes)

Replace `FlowView.vue` (3,559 lines) with a feature folder:

```
web/agentd-ui/src/
  views/WarppView.vue            — shell: layout, routing, save/run actions
  components/warpp/
    CanvasPane.vue               — Vue Flow instance, edge derivation, drag-connect
    CatalogPanel.vue             — node palette from GET /catalog, search
    NodeCard.vue                 — node rendering, typed handles, status chip
    PortHandle.vue               — colored by port type; dims incompatible targets mid-drag
    InspectorPanel.vue           — manifest-driven widgets for unwired ports
    MapRegion.vue                — Map container (Vue Flow parent/child)
    RunOverlay.vue               — SSE-driven statuses + output values on wires
    RunTimeline.vue              — event list for a run
  stores/warppEditor.ts          — document state, undo, dirty tracking
  stores/warppRun.ts             — run/event state
  api/warpp.ts                   — API client
  types/warpp.ts                 — document/manifest/event types
```

- Connection validation uses the catalog's coercion table — the canvas cannot
  produce a wire the backend would reject.
- Wiring a port hides its widget; unwiring restores it (with last literal).
- Widgets by type: text field, number input, boolean toggle, JSON editor,
  file path field, list editor.
- Sticky notes and groups persist as canvas-sidecar cosmetics, unchanged.
- No `ExpressionPicker`, no `ParameterFormField`, no expression UI of any kind.

## 12. Deletions (clean break)

- `internal/flow/` (all), `internal/agentd/flow_v2_execution.go`,
  `flow_v2_eval.go`, `flow_v2_expression.go`, `flow_v2_runtime.go`,
  `handlers_flow_v2.go`, `warpp_tools.go` and their tests
- `internal/tools/warpptool/`
- `web/agentd-ui/src/views/FlowView.vue`, `lib/flowEditorCompat.ts`,
  `components/flow/` (all), `types/flow*.ts`, `constants/flowNodes.ts`,
  `stores/flowRun.ts`, `api/flow.ts`
- Legacy `${A.*}` handling anywhere it remains
- `docs/warpp.md` → rewritten for the new model
- DB migration drops flow v2 workflow tables

Routes `/api/flows/v2/*` are removed, not aliased.

## 13. Testing

Backend (TDD):
- **Validator golden tests:** document in → diagnostics out (unique IDs, bad
  refs, type mismatch, unresolved `T`, cycles incl. Map outer-refs, variadic
  binding shapes, dynamic `as` resolution).
- **Engine semantics:** diamond join; `logic.if` fires one branch and the
  other cascade-skips; `logic.coalesce` rejoins; optional-input default rule;
  Map fan-out with `on_item_error` fail and skip, order preservation, nesting,
  lexical outer refs; retries/backoff; timeout; `completed_with_skips`.
- **Durable:** mid-run resume replays checkpoints, Map iteration keys.
- **Adapters:** one contract test per curated manifest (arg mapping, output
  mapping, contract-violation error); `tool.generic` round-trip.
- **Agent tools:** publish → schema derivation → invoke → declared outputs.

Frontend:
- Type-compat unit tests driven by a catalog fixture (same cases as backend
  coercion tests).
- Inspector widget rendering per port type; widget↔wire toggling.
- One e2e: build a two-node flow in the editor → save → run → SSE statuses
  appear on nodes.

## 14. Non-goals

- Schedule/webhook/event triggers (removed; revisit later if needed).
- Cycles/loops other than `control.map`.
- Expressions or per-wire transforms of any kind.
- n8n/ComfyUI import, legacy workflow migration.
- Curating manifests for the entire tool registry (10 + generic at launch).
