# Architecture Decisions: lexminify

Working product/architecture choices for lexical provider compression. Status dates refer to the 2026-07-14 implementation completion window.

## D1 — Enable level 6 (L5Aggressive) on product construction paths

**Statement:** Production/default engine constructions assign `LexMinifyLevel = agent.DefaultLexMinifyLevel` where `DefaultLexMinifyLevel = 6` (`lexminify.L5Aggressive`). Bare test `Engine{}` remains 0 unless set.

**Rationale:** Strongest progressive density on prose-heavy runtime context + histories; user directive to leave enabled at 6 and revert only if issues appear.

**Evidence:**

- `internal/agent/engine.go` constant + field comments
- Assignments in `app_init_services.go`, `chat/builders.go`, `delegator.go`, `agent_call.go`, `cmd/agent/main.go`

**Rejected alternatives:**

- Default off awaiting flags: rejected for product evaluation of savings immediately on provider path
- Default level < 6: rejected by explicit product instruction

## D2 — Zone bitmask with package DefaultZones (Zones==0)

**Statement:** `Options.Zones` / `Engine.LexMinifyZones` of zero resolves to package `DefaultZones`, not “all zones” and not “no zones”.

**Rationale:** Zero-value structs stay usable; construction sites only set level unless they need a custom mask.

**Evidence:** `MinifyMessages` zero-zone branch; engine field comment on zero → package default.

## D3 — DefaultZones includes ZoneSystemPrompt

**Statement:** System/developer prefix minifies under defaults for testing/eval of KV impact; operators can clear the bit for stable prefixes.

**Evidence:** `DefaultZones` const; `TestMessagesZonesMinifySystemByDefault`, `TestSystemPromptZoneCanBeDisabled`.

**Rejected alternatives:**

- Hard-never-touch static prefix: superseded; now headless under bit control

## D4 — DefaultZones includes ZoneTool (tool results minified by default)

**Statement:** Tool-role payloads are minified by default at the same level as other default zones. Blended NL + JSON tool dumps only transform unprotected prose; structure is protected.

**Rationale:** Tool results are a large fraction of multi-step agent context; pre-existing tool branch only needed the default bit. Protect scanner already covers common structured shapes, so a second level direction was deferred.

**Evidence:**

- `DefaultZones = … | ZoneTool | …` in `lexminify.go`
- `case role == "tool"` branch
- `TestMessagesZonesMinifyToolByDefault`, `TestToolZoneCanBeDisabled`
- Engine zero-zones comment lists “runtime + history + tool + assistant + system prompt”

**Rejected alternatives:**

- Tool off by default: superseded; default bit flipped on after system-prompt experiment pattern
- Per-tool separate max level (`LexMinifyToolMax`): rejected — adds API surface without measured need while span protection covers structured risk

**How to opt out:**

```go
eng.LexMinifyZones = int(
    lexminify.ZoneRuntimeContext |
    lexminify.ZoneHistory |
    lexminify.ZoneAssistant |
    lexminify.ZoneSystemPrompt,
)
```

## D5 — Provider-visible copy only

**Statement:** Minification never mutates permanent conversation history used for tool dispatch, checkpoints, or turn persistence. All hooks run on a provider-bound view.

**Evidence:** `lexMinifyForProvider` docstring; `MinifyMessages` shallow content copy; harness comments + `applyProviderContentToHarness`.

## D6 — Current request remains opt-in / light

**Statement:** `ZoneCurrentRequest` is **not** in `DefaultZones`. When enabled, default max level is `L0Whitespace` unless raised by options/engine, always clamped ≤ global level.

**Rationale:** Live user ask should stay recoverable; densest compression targets bulk context.

## Change Log (zone defaults)

1. Earlier: runtime + history (+ assistant only)
2. Then: + `ZoneSystemPrompt`
3. Current: + `ZoneTool` →  
   `ZoneRuntimeContext | ZoneHistory | ZoneTool | ZoneAssistant | ZoneSystemPrompt`

## D7 — Server-config gate; product default fully off

**Statement:** Lex minify is controlled by top-level `lexMinify` config (`enabled`, `level`, `zones`, `currentRequestMaxLevel`). The entire feature is **disabled by default** (`enabled: false` → engine level 0). Operators enable via `config.yaml` or Settings UI. When `enabled: true` and `level` is left `0`, loader/settings fill `RecommendedLexMinifyLevel` (6).

**Rationale:** Working-tree evaluation previously hard-coded level 6 on product construction paths; product requirement is opt-in configurability with safe zero defaults.

**Evidence:**

- `internal/config.LexMinifyConfig`, `EffectiveLevel`, `EngineSettings`
- Engine wiring through `app_init_services`, `chat/builders`, `cmd/agent`, delegated `SetLexMinify`
- Settings API fields `lexMinifyEnabled` / `lexMinifyLevel` / zones / current max
- UI section **Lex Minify**

**Rejected alternatives:**

- Keep package-constant hard default 6 on all product engines: rejected (user asked default feature disabled)
- Global only level without enable master: rejected — enable must be explicit

