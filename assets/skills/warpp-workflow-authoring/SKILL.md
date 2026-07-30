---
name: warpp-workflow-authoring
description: Author WARPP workflows for Manifold as typed-port dataflow JSON. Use when a user asks to build, create, design, generate, or edit a Flow / workflow / automation / pipeline / DAG in Manifold, or mentions WARPP, workflow nodes, or the /flow builder. Covers the document format, port types and coercions, the builtin node catalog, control flow (if/coalesce/map), and the workflow_catalog then workflow_save validate-and-self-correct loop.
metadata:
  short-description: Build and validate WARPP workflow JSON documents for Manifold.
---

# Authoring WARPP Workflows

WARPP is Manifold's typed-port dataflow engine. A workflow is a directed
acyclic graph of nodes. **Data moves only through typed wires or literals —
there are no expressions.** Your job is to produce a valid workflow *document*
(JSON) and prove it valid with `workflow_save`.

## The one rule

Every node is a pure function with declared, typed input and output ports. An
input binding is **exactly one of**:

- `{"from": "nodeID.port"}` — a wire from an upstream node's output port
- `{"value": <literal>}` — a fixed literal typed into the node

Never both. Never an expression string, template, or `${...}`. To reshape data,
use a data node (`data.extract`, `data.template`, `data.parse`, …).

## The authoring loop (do this every time)

1. **Discover tools with `tool_search`.** For every step that performs an
   external action (web search/fetch, file I/O, calling an API or another
   agent, sending a message, RAG, an MCP capability, …), call `tool_search`
   with a natural-language description of the capability you need — e.g.
   `"search the web"`, `"read a file"`, `"query a database"`,
   `"send a slack message"`. The user may have configured MCP servers that
   publish tools you cannot know ahead of time, so **do not assume a tool
   exists or guess its name** — search, then pick the best-matching result for
   the step. If several match, choose the most specific/appropriate one.
2. **`workflow_catalog`** — fetch the live node manifests for the tools you
   chose (plus the builtins), their exact port names/types, and the coercion
   table. Every registry tool is available as a node typed `tool.<name>`.
   **Never guess a node type or a port name — read them from the catalog.**
3. **Build the document** (shape below). Prefer wiring (`from`) over literals
   for anything that should be dynamic.
4. **`workflow_save`** — it validates and returns diagnostics. If any come
   back, fix each one (they carry a `code`, `message`, and `path`) and save
   again. Do not stop until it saves with no error diagnostics.
5. **`workflow_run`** (only if asked to test) — runs it and returns the
   declared outputs.

Discover → build → validate → self-correct. Do not hand back an unvalidated
document, and do not invent a tool you did not find via `tool_search` or the
catalog.

## Document shape

```json
{
  "id": "research-brief",
  "name": "Research brief",
  "inputs": [{ "name": "topic", "type": "text", "required": true }],
  "nodes": [
    { "id": "search", "type": "tool.web_search",
      "inputs": { "query": { "from": "in.topic" }, "max_results": { "value": 5 } } },
    { "id": "summary", "type": "llm.generate",
      "inputs": { "instruction": { "value": "Summarize as 5 bullets." },
                  "input": { "from": "search.results_text" } } }
  ],
  "outputs": { "brief": { "from": "summary.text" } }
}
```

- `id` matches `^[a-zA-Z0-9_-]+$`; `name` is human-readable.
- `inputs` are the workflow's typed parameters, exposed to nodes as
  `in.<name>` (e.g. `{"from": "in.topic"}`).
- Each node has a unique `id`, a `type` from the catalog, and an `inputs` map
  keyed by the manifest's input port names.
- `outputs` maps result names to node output ports; this is the run result.
- `in` and `item` are reserved node ids.
- Optional: `settings.max_concurrency` (default 4); `publish.tool: true` to
  expose the workflow as an agent tool; `project_id` to scope file tools to a
  project.

## Types and coercions

Port types: `text`, `number`, `boolean`, `json`, `file`, `list<T>` (T is any
scalar). The **only** implicit coercions, applied to wires, are:

- `number → text`
- `boolean → text`

Nothing else coerces. `json → text` needs `data.stringify`; `text → json` needs
`data.parse`; pull a field out of `json` with `data.extract`. **Literals must
match the port type exactly** (`"5"` for a text port, `5` for a number port). A
`file` port accepts a literal path string.

## Builtin nodes (stable; always available)

| Type | Inputs | Outputs |
|---|---|---|
| `data.extract` | `source: json`, `path: text`, `as: text` (default `"json"`) | `value` (typed by `as`) |
| `data.template` | `template: text`, `vars` (named, any type) | `text: text` |
| `data.merge` | `objects` (list of `json`) | `json: json` |
| `data.stringify` | `value: T` | `text: text` |
| `data.parse` | `text: text` | `json: json` |
| `data.constant` | `value: json`, `as: text` (default `"json"`) | `value` (typed by `as`) |
| `logic.if` | `condition: boolean`, `value: T` | `then: T`, `else: T` (one fires) |
| `logic.coalesce` | `values` (list of `T`) | `value: T` (first that fired) |
| `logic.equals` | `a: T`, `b: T` | `result: boolean` |
| `logic.contains` | `haystack: text`, `needle: text` | `result: boolean` |
| `logic.not` | `value: boolean` | `result: boolean` |
| `logic.greater_than` | `a: number`, `b: number` | `result: boolean` |
| `control.map` | `items: list<T>`, `concurrency: number`, `on_item_error: text` | `results: list<…>` |
| `llm.generate` | `instruction: text`, `input: text`, `model: text` | `text: text` |

**Tool nodes** are `tool.<name>` — one for every tool in the registry,
including MCP-server tools (e.g. `tool.web_search`, `tool.file_write`,
`tool.brave_brave_web_search`). The set is instance-specific and not known in
advance, so **find tools with `tool_search`** (by capability) rather than
assuming; then use the match as node type `tool.<its name>`. Each tool node has
a `result: json` output plus, for curated tools, typed outputs. Get the exact
input/output ports from `workflow_catalog`.

`data.extract` / `data.constant` set their output type via the `as` config:
one of `text`, `number`, `boolean`, `json`, `list<json>`.

## Named-variadic and list-variadic ports

- `data.template.vars` is **named-variadic**: an object of bindings, one per
  `{placeholder}` in the template —
  `"vars": { "topic": {"from":"in.topic"}, "n": {"value": 3} }`.
- `logic.coalesce.values` and `data.merge.objects` are **list-variadic**: an
  array of bindings — `"values": [ {"from":"a.text"}, {"from":"b.text"} ]`.

## Control flow (gated wires, not conditionals)

- `logic.if` fires **exactly one** of `then` / `else` per run based on
  `condition`.
- A node whose **required** input never fires (e.g. it reads the branch that
  didn't fire) is **skipped**, and skips cascade downstream. An **optional**
  input whose source is skipped falls back to its default.
- `logic.coalesce` rejoins branches: it emits the first of its inputs that
  actually fired.
- `control.map` fans out over `items: list<T>`: its `body` subgraph runs once
  per item, seeing `item.value` and `item.index`, and gathers the body's single
  `result` output into `results`. `on_item_error` is `fail` (default) or
  `skip`. Bodies may also wire from outer / `in` ports.

## Common mistakes to avoid

- Assuming a tool exists or guessing its name. Use `tool_search` to find the
  right tool (MCP servers add tools you can't know in advance), then read its
  exact ports from `workflow_catalog`.
- Using an expression / template string in a binding. Use a `data.*` node.
- Wiring `json` into a `text` port (or vice-versa) without a
  `data.stringify` / `data.parse` / `data.extract` between them.
- A literal whose JSON type doesn't match the port (e.g. `5` into a text port).
- Forgetting `data.template.vars` is a named object and
  `logic.coalesce.values` is an array.
- Returning a document without running it through `workflow_save`.

## More examples

Read `references/examples.md` (via `skill_read`, path `references/examples.md`)
for full worked documents: a linear pipeline, an if/coalesce branch, and a map
fan-out.
