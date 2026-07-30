# WARPP Workflows

WARPP is a typed-port dataflow engine. A workflow is a directed acyclic graph
of nodes; **data moves only through typed wires**. There are no expressions, no
shared attribute bag, and no per-wire transforms — the model ComfyUI uses.
Reshaping data is done by small, visible data nodes.

## The one rule

Every node is a pure function with declared, typed input and output ports. An
input port is **either** wired to exactly one upstream output port **or** holds
a literal value — never both, never an expression string. "What does this node
output" is answered by its manifest before anything runs.

## The document

A workflow is one JSON document. Wiring lives inside each input binding; there
is no separate edges array (the canvas derives edges from `from` references).

```json
{
  "id": "lin",
  "name": "Linear",
  "inputs": [{ "name": "topic", "type": "text", "required": true }],
  "nodes": [
    {
      "id": "tpl",
      "type": "data.template",
      "inputs": {
        "template": { "value": "about {t}" },
        "vars": { "t": { "from": "in.topic" } }
      }
    }
  ],
  "outputs": { "out": { "from": "tpl.text" } }
}
```

- An input binding is exactly one of `{"from": "nodeID.port"}` or
  `{"value": <literal>}`.
- `in` is a virtual node exposing the workflow's own typed inputs
  (`in.<name>`). `item` is the virtual node inside a Map body
  (`item.value`, `item.index`).
- `outputs` binds names to node output ports; that map is the run result.
- Node ids match `^[a-zA-Z0-9_-]+$`; `in` and `item` are reserved.
- `settings.max_concurrency` caps parallel node execution (default 4).
- `settings.default_policy` sets per-node defaults (see Execution).
- `publish.tool: true` exposes the workflow as a callable agent tool.
- Optional `project_id` scopes filesystem tools to a project sandbox.

## Types

Port types: `text`, `number`, `boolean`, `json`, `file`, and `list<T>` where
`T` is any of the scalar types.

The **complete** implicit coercion table, applied only to wired connections:

| From | To |
|---|---|
| `number` | `text` |
| `boolean` | `text` |

Nothing else coerces implicitly. `json → text` needs `data.stringify`;
`text → json` needs `data.parse`. Literals must match their port type exactly.

Built-in logic/data nodes may declare a single type variable `T` (optionally
`list<T>`), unified from the connected wires. `data.extract` and
`data.constant` type their output from an `as` config value
(`text`/`number`/`boolean`/`json`/`list<json>`).

## Built-in nodes

Data nodes:

| Type | Inputs | Outputs |
|---|---|---|
| `data.extract` | `source: json`, `path: text`, `as` (config) | `value` (typed by `as`) |
| `data.template` | `template: text`, `vars` (named, any type) | `text: text` |
| `data.merge` | `objects` (list of `json`) | `json: json` |
| `data.stringify` | `value: T` | `text: text` |
| `data.parse` | `text: text` | `json: json` |
| `data.constant` | `value: json`, `as` (config) | `value` (typed by `as`) |

Logic nodes:

| Type | Inputs | Outputs |
|---|---|---|
| `logic.if` | `condition: boolean`, `value: T` | `then: T`, `else: T` (one fires) |
| `logic.coalesce` | `values` (list of `T`) | `value: T` (first that fired) |
| `logic.equals` | `a: T`, `b: T` | `result: boolean` |
| `logic.contains` | `haystack: text`, `needle: text` | `result: boolean` |
| `logic.not` | `value: boolean` | `result: boolean` |
| `logic.greater_than` | `a: number`, `b: number` | `result: boolean` |

LLM node:

| Type | Inputs | Outputs |
|---|---|---|
| `llm.generate` | `instruction: text`, `input: text`, `model: text` | `text: text` |

## Control: Map (fan-out)

`control.map` runs a nested body subgraph once per item and gathers the body's
single `result` output into a list. The body sees `item.value` (one element)
and `item.index`, and may also reference any port visible to the Map node
(lexical scope).

```json
{
  "id": "m",
  "name": "Map",
  "inputs": [
    { "name": "names", "type": "list<text>", "required": true },
    { "name": "suffix", "type": "text", "required": true }
  ],
  "nodes": [
    {
      "id": "per",
      "type": "control.map",
      "inputs": {
        "items": { "from": "in.names" },
        "concurrency": { "value": 3 },
        "on_item_error": { "value": "skip" }
      },
      "body": {
        "nodes": [
          {
            "id": "t",
            "type": "data.template",
            "inputs": {
              "template": { "value": "{n}-{s}" },
              "vars": { "n": { "from": "item.value" }, "s": { "from": "in.suffix" } }
            }
          }
        ],
        "outputs": { "result": { "from": "t.text" } }
      }
    }
  ],
  "outputs": { "all": { "from": "per.results" } }
}
```

Iterations run concurrently up to `concurrency`. `on_item_error` is `fail`
(default; first failure fails the run) or `skip` (drop the item, keep the
rest). Maps nest.

## Tool nodes

Registry tools are exposed as curated adapter nodes. Each declares typed
output ports and also a `raw: json` port carrying the tool's whole result.

| Node type | Key outputs |
|---|---|
| `tool.web_search` | `results: list<json>`, `results_text: text` |
| `tool.web_fetch` | `markdown: text`, `url: text` |
| `tool.file_read` | `content: text` |
| `tool.file_write` | `path: file` |
| `tool.run_cli` | `stdout: text`, `exit_code: number` |
| `tool.rag_retrieve` | `results: list<json>` |
| `tool.rag_ingest` | `raw: json` |
| `tool.agent_call` | `text: text` |
| `tool.matrix_room_message` | `raw: json` |

**Every other registered tool is auto-exposed.** At catalog time each tool in
the registry that lacks a curated adapter — including all MCP-server tools —
is turned into a `tool.<name>` node whose input ports are derived from the
tool's JSON schema (string→text, number/integer→number, boolean→boolean,
array→list<…>, object→json) and whose output is `result: json`. MCP tools
appear under names like `tool.brave_brave_web_search` or
`tool.paper-search_search_arxiv`. Tools with a free-form schema expose a single
`args: json` input. Derivation reflects the live registry, so tools from
MCP servers that connect after boot appear automatically.

**Escape hatch:** `tool.generic` takes `tool: text` and `args: json`, returns
`result: json`. Use it for anything you'd rather call by raw name; combine with
`data.extract` to reshape a tool's `result`.

## Execution semantics

- A node runs at most once per run (per Map iteration): it gathers all inputs,
  executes, and emits typed values on its output ports.
- **Skip rule:** if a *required* input's source is skipped or never fires (for
  example the untaken branch of `logic.if`), the node is skipped, and skips
  cascade downstream. If an *optional* input's source is skipped, the node runs
  with the port's default.
- **Node policy** (`policy`, with `settings.default_policy` as fallback):
  `timeout` (Go duration), `retries { max, backoff: fixed|exponential }`,
  `on_error: fail | skip`.
- **Run statuses:** `running`, `completed`, `completed_with_skips`, `failed`,
  `cancelled`.
- Runs are durable and resumable; each node checkpoints, and runs survive
  restarts.

## HTTP API

| Method & path | Purpose |
|---|---|
| `GET /api/warpp/workflows` | List workflow summaries |
| `GET /api/warpp/workflows/{id}` | Document + canvas sidecar |
| `PUT /api/warpp/workflows/{id}` | Validate + save (400 with diagnostics on error) |
| `DELETE /api/warpp/workflows/{id}` | Delete |
| `POST /api/warpp/validate` | Diagnostics for a document without saving |
| `POST /api/warpp/runs` | Start a run: `{workflow_id, input}` → `{run_id}` |
| `GET /api/warpp/runs/{id}/events` | SSE stream or JSON snapshot (Accept-negotiated) |
| `GET /api/warpp/catalog` | Node manifests + coercion table + published workflows |

Run events (SSE + JSON): `run_started`, `run_completed`, `run_failed`,
`run_cancelled`, `node_started`, `node_completed`, `node_failed`,
`node_skipped`, `node_retrying`. Node events carry a `node_path` (Map
iterations look like `per[2].t`); `node_completed` carries the node's typed
output values.

## Agent tools

The chat agent gets workflow authoring tools: `workflow_catalog` (manifests +
coercions), `workflow_list`, `workflow_get`, `workflow_save` (validate + save;
returns diagnostics on failure so the model can self-correct), and
`workflow_run`. Workflows saved with `publish.tool: true` are additionally
registered as callable tools named `warpp_<id>`, whose JSON schema is derived
from the workflow's input ports and whose result is exactly the declared
outputs.

## Workflows as nodes (subflows)

A saved workflow is itself a node manifest: its inputs are input ports and its
outputs are output ports. Place another workflow as a node with
`type: "flow.<workflow-id>"` to compose them. Recursive inclusion is rejected
at save time.

## Removed

The legacy WARPP runner, Flow v2, `${A.*}` attribute templating,
`={{...}}`/`$node`/`$run` expressions, edge field mappings, and the
`/api/flows/v2/*` routes have been removed. There is no migration; workflows
are authored fresh in the new model.
