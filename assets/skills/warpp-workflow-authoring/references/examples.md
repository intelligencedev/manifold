# WARPP workflow examples

Three complete, valid documents covering the common shapes. All use only
builtin nodes plus `tool.web_search` / `llm.generate`, so they validate on any
instance that has those tools. Confirm the exact ports of any tool node with
`workflow_catalog` before adapting.

## 1. Linear pipeline (literal value entry + one wire)

Combines two literal constants and a workflow input into a prompt string.
Demonstrates literal entry (`value`), typed constants (`as`), and
named-variadic template vars.

```json
{
  "id": "intro-builder",
  "name": "Intro builder",
  "inputs": [{ "name": "audience", "type": "text", "required": true }],
  "nodes": [
    { "id": "tone", "type": "data.constant",
      "inputs": { "value": { "value": "warm and concise" }, "as": { "value": "text" } } },
    { "id": "bullets", "type": "data.constant",
      "inputs": { "value": { "value": 3 }, "as": { "value": "number" } } },
    { "id": "prompt", "type": "data.template",
      "inputs": {
        "template": { "value": "Write a {tone} intro for {audience}. Include {n} bullet points." },
        "vars": {
          "tone": { "from": "tone.value" },
          "audience": { "from": "in.audience" },
          "n": { "from": "bullets.value" }
        }
      } }
  ],
  "outputs": { "prompt": { "from": "prompt.text" } }
}
```

## 2. Branch with if / coalesce

Routes on whether the topic contains "urgent". Each branch builds a different
prompt; `coalesce` rejoins them. The untaken branch's nodes are skipped
automatically; `coalesce` emits whichever fired.

```json
{
  "id": "triage-note",
  "name": "Triage note",
  "inputs": [{ "name": "topic", "type": "text", "required": true }],
  "nodes": [
    { "id": "isUrgent", "type": "logic.contains",
      "inputs": { "haystack": { "from": "in.topic" }, "needle": { "value": "urgent" } } },
    { "id": "gate", "type": "logic.if",
      "inputs": { "condition": { "from": "isUrgent.result" }, "value": { "from": "in.topic" } } },
    { "id": "urgentMsg", "type": "data.template",
      "inputs": { "template": { "value": "ESCALATE: {t}" }, "vars": { "t": { "from": "gate.then" } } } },
    { "id": "normalMsg", "type": "data.template",
      "inputs": { "template": { "value": "Queued: {t}" }, "vars": { "t": { "from": "gate.else" } } } },
    { "id": "note", "type": "logic.coalesce",
      "inputs": { "values": [ { "from": "urgentMsg.text" }, { "from": "normalMsg.text" } ] } }
  ],
  "outputs": { "note": { "from": "note.value" } }
}
```

## 3. Map fan-out (per-item subgraph)

Runs the body once per URL, fetching each and returning a list of texts. The
body sees `item.value` (one element) and produces a single `result`, gathered
into the map's `results` list.

```json
{
  "id": "fetch-all",
  "name": "Fetch all",
  "inputs": [{ "name": "urls", "type": "list<text>", "required": true }],
  "nodes": [
    { "id": "each", "type": "control.map",
      "inputs": {
        "items": { "from": "in.urls" },
        "concurrency": { "value": 3 },
        "on_item_error": { "value": "skip" }
      },
      "body": {
        "nodes": [
          { "id": "fetch", "type": "tool.web_fetch",
            "inputs": { "url": { "from": "item.value" } } }
        ],
        "outputs": { "result": { "from": "fetch.markdown" } }
      } }
  ],
  "outputs": { "pages": { "from": "each.results" } }
}
```

## Reshaping data with `data.extract`

When a tool returns `result: json` and you need one field as text, put an
`data.extract` between them:

```json
{ "id": "title", "type": "data.extract",
  "inputs": {
    "source": { "from": "search.result" },
    "path": { "value": "results.0.title" },
    "as": { "value": "text" }
  } }
```

`path` is a dot/index path (`results.0.title`). `as` sets the output type so
the extracted value can be wired into a typed port.
