# WARPP workflows (WARPP)

Overview

WARPP workflows are lightweight JSON templates that describe a sequence of human-readable steps which call out to registered tools. The WARPP runner loads workflows from `configs/workflows` and executes them in-process using the configured tool registry.

A workflow maps an intent (a short name) to a set of ordered steps. Each step can have an optional guard expression and a tool reference. Steps may optionally publish their result to Kafka when `publish_result` is set.

Why use WARPP

- Simple, testable workflow templates.
- Decouples orchestration (message input / Kafka) from business logic (tools).
- Allows LLM-driven or tool-driven orchestration using existing tool implementations.

JSON template: top-level shape

A workflow JSON has this top-level structure:

- `intent` (string) - a short name used to select the workflow.
- `description` (string) - a human description of the workflow's purpose.
- `keywords` (array of string) - used by the simple detector to pick a workflow from a short utterance.
- `steps` (array of Step objects) - the sequential steps that the runner will execute.

Basic example (noop)

```json
{
  "intent": "noop",
  "description": "Return a greeting.",
  "keywords": ["noop"],
  "steps": [
    {
      "id": "s1",
      "text": "Invoke an LLM",
      "tool": {
        "name": "llm_transform",
        "args": { "instruction": "", "input": "${A.query}" }
      },
      "publish_result": true
    }
  ]
}
```

Step object fields

- `id` (string, required): unique identifier for the step within the workflow.
- `text` (string): human-readable description of the step (used in execution summary/logs).
- `guard` (string, optional): a boolean expression evaluated against the current attributes map; if the guard evaluates false, the step is skipped.
- `tool` (object, optional): a `ToolRef` describing which tool to call and with which args.
- `publish_result` (boolean, optional): if true, the WARPP runner will invoke the configured publisher after the step completes, allowing the orchestrator to produce a Kafka message containing the step's output.

ToolRef

A ToolRef identifies a tool in the tool registry and provides arguments for the tool call.

- `name` (string): the tool name (must exist in the configured tools registry).
- `args` (object): a map of parameters passed to the tool. Values may be strings, arrays, or nested maps and will be processed by the templating logic described below.

Attributes and templating

- `Attrs` (map[string]any) is the shape used to pass contextual data into workflows and between steps.
- Before execution, the runner calls `Personalize` to infer a few basic attributes and to filter steps by guards. The runner sets:
  - `A["utter"]` — preferred entry field. If not set, the runner will examine `echo` or `query` as fallbacks.
  - `A["query"]` — set from `A["utter"]` and used by templates like `${A.query}`.
  - `A["os"]` — runtime OS (e.g., `darwin`, `linux`, `windows`).

Templating rules

- Templating is a simple, synchronous substitution performed on string values in tool `args`.
- Syntax: `${A.key}` — replaced with the string form of `A["key"]` during `Execute`.
- The renderer walks nested arrays and maps, substituting string placeholders wherever they appear.

Guards

- Guards are simple boolean expressions evaluated against `Attrs` using a lightweight evaluator. Typical guard forms used in the default workflows are existence checks like `A.first_url` or boolean comparisons like `A.os == 'windows'`.
- Guards are evaluated in `Personalize` when building the trimmed, executable workflow.

Execution flow (high-level)

1. Orchestrator receives a command message (see `docs/kafka.md` for message format).
2. The orchestrator looks up the requested workflow by `workflow`/intent and calls the WARPP adapter.
3. WARPP `Personalize` is called with provided `Attrs` to infer `query`/`utter` and to trim steps by guard.
4. WARPP `Execute` runs the steps in order:
   - For each step with a `tool`, WARPP renders the `args` by substituting `${A.*}` placeholders.
   - WARPP dispatches the tool call via the tool registry.
   - Tool output is recorded in attributes (`A.last_payload`, `A.payloads`) and may be used by later steps.
   - If `publish_result` is true on the step and a publisher callback is supplied by the caller, the runner will invoke the publisher with the step ID and raw payload.
5. When execution ends, WARPP returns a human summary string which is converted to a JSON result by the adapter.

Tool interaction and payloads

- Tools are registered into a `tools.Registry` and must implement a dispatchable contract (the project `internal/tools` code manages this).
- Tool results are JSON payloads (raw bytes). The runner records the payload as a string in `A.last_payload` and appends to `A.payloads`.
- Tools may set structured data in attributes by returning a JSON object that the workflow's subsequent steps can inspect via templates or guards.

Errors and retry behavior

- If a tool call returns an error, WARPP `Execute` returns that error to the caller. The orchestrator treats certain errors as transient and will retry the entire workflow execution a limited number of times.
- If a step's publish to Kafka fails (publisher returns error), the runner treats publishing as best-effort and continues execution (the orchestrator-level publisher chooses whether to surface errors or log them).

Best practices

- Use clear `id` values for steps so downstream consumers of per-step messages can identify them.
- Keep `args` small and pre-render values where possible; LLM outputs can be large and are stored in attributes as strings.
- Prefer setting the user's text in `Attrs.query` (or `Attrs.utter`) in command envelopes to make templating predictable.
- Use `publish_result` selectively — publishing every step can produce a lot of traffic.

Examples

- `configs/workflows/noop.json` demonstrates a minimal LLM step with `publish_result`.

Invocation examples

- Run a WARPP workflow directly with the agent CLI (WARPP mode). The `-warpp` flag tells the
  agent to use the WARPP runner and select a workflow by intent using the built-in detector. You
  may also pass an explicit intent after the `-warpp` flag to force a specific workflow.

  Example: run the `noop` workflow by intent (explicit):

  ./dist/agent -q "hi" -warpp noop

  Example output:

  WARPP: executing intent noop
  - Textbox
  - Invoke an LLM
  - Textbox

  Objective complete. (steps=3).

- Run WARPP mode with automatic intent detection. The runner picks the best matching
  workflow based on `keywords` in each workflow JSON. If no keywords match, a default
  is chosen.

  ./dist/agent -q "save latest web search to a file" -warpp

- Orchestrator / Kafka-driven execution. The orchestrator loads workflows from
  `configs/workflows` and exposes an adapter that executes a named workflow (intent). When
  the orchestrator receives a command envelope, it looks up the `workflow` field and runs
  that workflow via the WARPP adapter. See `cmd/orchestrator/main.go` for the wiring.

Routes vs WARPP workflows

- The `routes:` block in `config.yaml` maps simple substring/regex rules to a "specialist"
  name (an inference-only endpoint). These specialist routes are evaluated by
  `specialists.Route(...)` and, when matched, the agent will attempt to call a configured
  specialist with that name. They do NOT automatically trigger a WARPP workflow.

  For example, this `config.yaml` fragment:

  routes:
  - name: web_to_file
    contains: ["webwrite"]
    regex: ["(?i)webwrite"]

  will cause the pre-dispatch router to return the name `web_to_file` when the user text
  contains "webwrite". The agent then expects a specialist called `web_to_file` and will
  dispatch to it. If no such specialist is configured, the agent logs that the specialist
  was not found and continues with the normal agent flow — it will not run the
  `configs/workflows/web_to_file.json` workflow automatically.

  If you want routes to kick off WARPP workflows, you can either:
  - Configure a specialist with the same name that proxies to a workflow, or
  - Modify the pre-dispatch logic to call the WARPP runner when a route name matches a
    workflow intent (small code change in `cmd/agent/main.go`).

Troubleshooting

- If a placeholder `${A.key}` is empty in the dispatched tool args, inspect the incoming command `Attrs` and the `Personalize` logic.
- If a step is unexpectedly skipped, evaluate the `guard` expression against the attribute map; a missing attribute will cause existence guards to fail.


---

For more on Kafka message shapes and how the orchestrator integrates with Kafka, see `docs/kafka.md`.
