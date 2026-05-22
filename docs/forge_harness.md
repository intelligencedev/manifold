# Forge Harness

The Forge harness is an optional guarded agent loop for Manifold. It keeps the existing Manifold runtime, providers, tools, memory systems, policy checks, delegation, callbacks, and chat plumbing, but adds Forge-style validation around each model/tool step.

The harness is disabled by default. Existing Manifold chat behavior remains the default path unless `harness.enabled` is set to `true`.

## When To Use It

Use legacy mode when ordinary chat behavior matters most and the agent should be allowed to answer directly without extra validation.

Use guarded chat mode when you want the current chat semantics plus protection against unavailable tool calls and bounded recovery from tool execution errors. Guarded chat still permits final text responses.

Use workflow mode when the agent must follow a tool-driven process and finish through a terminal tool such as `agent_response`. In workflow mode, bare text is rejected until the terminal tool succeeds.

## Configuration

Top-level configuration applies to the orchestrator, specialists, and team engine construction. Config-file specialists, persisted specialists, per-user orchestrator overlays, and team orchestrators can each carry a `harness` block that overrides the top-level settings for that target.

```yaml
harness:
  enabled: false
  mode: guarded_chat # legacy | guarded_chat | workflow
  rescueEnabled: true
  maxRetriesPerStep: 3
  maxToolErrors: 2
  terminalTools:
    - agent_response
  requiredSteps: []
  toolPrerequisites: {}
  compact:
    enabled: true
    keepRecentSteps: 4
    phaseThresholds: [0.60, 0.75, 0.90]
```

Important fields:

- `enabled`: routes `Engine.Run` and `Engine.RunStream` through the guarded loop.
- `mode`: selects legacy, guarded chat, or workflow semantics.
- `rescueEnabled`: converts conservative structured JSON tool-call responses embedded in assistant text into native tool calls before validation.
- `maxRetriesPerStep`: number of validation retries for malformed/non-compliant model output.
- `maxToolErrors`: maximum consecutive structured tool failures before the run stops with a typed harness error.
- `terminalTools`: tools whose successful result can complete workflow mode. `agent_response` is the default.
- `requiredSteps`: tool names that must succeed before a terminal tool can complete the run.
- `toolPrerequisites`: per-tool prerequisites checked before dispatch.
- `compact`: enables deterministic harness compaction inside the guarded loop.

Per-specialist override example:

```yaml
specialists:
  - name: research_planner
    enableTools: true
    harness:
      enabled: true
      mode: workflow
      terminalTools: [agent_response]
      requiredSteps: [web_search, web_fetch]
      toolPrerequisites:
        web_fetch:
          - tool: web_search
            matchArg: query
```

## Workflow Prerequisites

Prerequisites are configured by destination tool. A name-only prerequisite requires any prior successful call to another tool:

```yaml
harness:
  enabled: true
  mode: workflow
  terminalTools: [agent_response]
  toolPrerequisites:
    summarize:
      - tool: fetch
```

An argument-matched prerequisite requires a prior successful call with the same JSON argument value:

```yaml
harness:
  enabled: true
  mode: workflow
  terminalTools: [agent_response]
  toolPrerequisites:
    fetch:
      - tool: search
        matchArg: url
```

The harness evaluates a whole tool-call batch against the pre-batch state. A batch that includes a premature terminal call or unmet prerequisite is rejected before any tool in that batch runs.

When workflow mode uses `agent_response` as a terminal tool, the engine overlays that utility tool for the scoped run if the active registry does not already expose it.

## OpenAI-Compatible Proxy

Manifold exposes a guarded `/v1/chat/completions` proxy for OpenAI-compatible clients. The proxy accepts OpenAI-style `messages` and `tools`, injects a hidden `agent_response` terminal tool, runs the harness validation/retry loop, and then:

- returns `agent_response` calls as plain assistant `content`;
- returns real client tool calls as normal OpenAI-compatible `tool_calls`;
- supports streaming responses as `chat.completion.chunk` server-sent events.

The proxy is independent from `/agent/run`; it does not execute client-supplied tools and does not alter Manifold chat persistence.

## Context Compaction

Harness compaction is deterministic and does not call an LLM. It operates on harness metadata before provider requests:

1. Drop old validation nudges and truncate old tool results.
2. Compact old tool results while preserving assistant tool-call skeletons.
3. Compact old assistant text/reasoning while retaining control-flow skeletons.

The harness keeps workflow progress in `StepTracker`, outside model-visible history. The compacted prompt may include a hint such as `Steps completed: ...`, but enforcement never depends on that text.

Existing rolling chat summarization still runs at turn boundaries. Harness compaction runs inside the guarded loop and is focused on long tool histories created during a single run.

## Streaming Behavior

When the harness is enabled for `RunStream`, streamed provider chunks are buffered until the completed assistant response passes validation. Accepted deltas and thought summaries are then replayed through the existing callbacks. Invalid streamed text is not emitted to callers.

Tool callbacks continue to use the existing Manifold callback surface:

- `OnAssistant`
- `OnDelta`
- `OnThoughtSummary`
- `OnToolStart`
- `OnTool`
- `OnTurnMessage`
- `OnSummaryTriggered`

## Verification

Run deterministic harness scenarios without live model calls:

```bash
make test-forge-harness
```

CI runs this deterministic target in addition to the full Go test suite.

This covers:

- basic two-step workflow
- sequential three-step workflow
- premature terminal recovery
- prerequisite violation recovery
- unknown tool recovery
- tool error recovery
- compaction stress

Build the Forge-tagged backend binary:

```bash
make build-forge
```

For broader regression coverage before merge, run:

```bash
go test ./internal/agent/... ./internal/config
go test ./internal/agentd
```

`internal/agentd` tests create local `httptest` listeners, so sandboxed environments may need elevated permissions for that package.

## Rollout

1. Keep the default `harness.enabled: false` for legacy behavior.
2. Enable guarded chat mode locally for one low-risk agent or specialist.
3. Run `make test-forge-harness` and selected manual chat runs.
4. Enable workflow mode only for agents with defined terminal tools and required steps.
5. Use per-specialist or team overrides when only one target should run with stricter workflow semantics.
