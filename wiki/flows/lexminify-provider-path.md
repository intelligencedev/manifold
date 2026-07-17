# Flow: Provider-Visible Lexical Minification

## Purpose

Show when `lexminify` runs relative to permanent conversation history and the provider call, including the tool-zone default and the unminified system advisory.

## Sequence

```mermaid
sequenceDiagram
  participant Loop as Engine loop / harness
  participant Hist as Permanent history
  participant Adapt as lexMinifyForProvider
  participant Pack as lexminify.MinifyMessages
  participant Prov as LLM.Chat / ChatStream / harness inference

  Loop->>Hist: mutate/append tool results, assistant turns
  Note over Hist: Never minified in place
  Loop->>Adapt: msgs copy path (same content refs input)
  Adapt->>Adapt: if LexMinifyLevel <= Off return msgs
  Adapt->>Pack: Options{Level, Zones, CurrentRequestMaxLevel}
  Pack->>Pack: Zones==0 => DefaultZones<br/>(incl ZoneTool + ZoneSystemPrompt)
  Pack->>Pack: system zone: strip prior notice, minify body,<br/>prepend LexMinifyAdvisory on first system/developer
  Pack-->>Adapt: Result{Messages copy, diagnostics}
  Adapt->>Adapt: log lexminify_applied if Changed
  Adapt-->>Loop: providerMsgs
  Loop->>Prov: Chat/ChatStream(providerMsgs) or harness serial view
```

## Hook Sites (evidence)

| Site | Binding |
| --- | --- |
| `internal/agent/engine_loop.go` | `providerMsgs := e.lexMinifyForProvider(ctx, msgs)` then `e.LLM.Chat` / metrics |
| Same file, stream path | `providerMsgs` then `e.LLM.ChatStream` |
| `internal/agent/engine_harness.go` | `providerSerial := e.lexMinifyForProvider(ctx, harness.SerializeMessages(...))` before inference; permanent `prerHistory` stays raw; minified content layered via `applyProviderContentToHarness` |

Comments in harness explicitly state permanent harness history stays uncompressed; only the provider-visible content view is densified.

## Message Role Routing

Inside `MinifyMessages` (order is branch logic, not list order):

1. System/developer (static prefix / those roles) → minify iff `ZoneSystemPrompt`. Body is minified **after** stripping any prior advisory; the first system/developer message is then re-wrapped with unminified `LexMinifyAdvisory` (`[LEXMINIFY NOTICE]…---`).
2. `tool` → minify iff `ZoneTool` (**default on**)
3. `assistant` → minify iff `(ZoneHistory|ZoneAssistant)` and not treated as live-only edge cases
4. `user` last-user → section-marker split (`RUNTIME CONTEXT` full level if zone; `CURRENT REQUEST` never minified)
5. Other user history → `ZoneHistory`

## Provider-Visible Shape (system zone on)

```
[system]
[LEXMINIFY NOTICE]
…plain-text explanation (never minified)…
---
…minified system / developer instructions…

[user]
[RUNTIME CONTEXT]
…may be minified…
[CURRENT REQUEST]
…user-typed prompt (never minified)…
```

## Default Product Behavior

- Level comes from `config.LexMinify.EngineSettings()` (feature **disabled by default** until `enabled: true`)
- Zones **DefaultZones** when `LexMinifyZones == 0`:
  - Minifies: runtime context, history, **tool results**, assistant history, system/developer prefix (with advisory)
  - Does **not** minify: live `[CURRENT REQUEST]` body;, does **not** minify the advisory
- Structured spans inside minified zones — including tool JSON fragments — pass through protect scanners unchanged

## Related Pages

- [lexminify component](../components/lexminify.md)
- [Decisions](../architecture/decisions.md)
