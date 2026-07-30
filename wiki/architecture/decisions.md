# Architecture Decisions: lexminify

Working product/architecture choices for lexical provider compression. Status dates refer to the 2026-07-14 implementation completion window and later follow-ups.

## D1 — Enable level 6 (L5Aggressive) on product construction paths

**Statement:** When the feature is enabled, product engines use progressive level 6 (`lexminify.L5Aggressive`) via `agent.DefaultLexMinifyLevel` / `RecommendedLexMinifyLevel`. Bare test `Engine{}` remains 0 unless set.

**Rationale:** Strongest progressive density on prose-heavy runtime context + histories.

**Evidence:**

- `internal/agent/engine.go` constant + field comments
- Config `RecommendedLexMinifyLevel` and engine construction sites

**Rejected alternatives:**

- Default off awaiting flags: superseded by D7 config master enable
- Default level < 6 when enabled: rejected for evaluation of max densification

## D2 — Zone bitmask with package DefaultZones (Zones==0)

**Statement:** `Options.Zones` / `Engine.LexMinifyZones` of zero resolves to package `DefaultZones`, not “all zones” and not “no zones”.

**Rationale:** Zero-value structs stay usable; construction sites only set level unless they need a custom mask.

**Evidence:** `MinifyMessages` zero-zone branch; engine field comment on zero → package default.

## D3 — DefaultZones includes ZoneSystemPrompt

**Statement:** System/developer prefix minifies under defaults; operators can clear the bit for stable prefixes.

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

**Rejected alternatives:**

- Tool off by default: superseded
- Per-tool separate max level (`LexMinifyToolMax`): rejected — adds API surface without measured need while span protection covers structured risk

## D5 — Provider-visible copy only

**Statement:** Minification never mutates permanent conversation history used for tool dispatch, checkpoints, or turn persistence. All hooks run on a provider-bound view.

**Evidence:** `lexMinifyForProvider` docstring; `MinifyMessages` shallow content copy; harness comments + `applyProviderContentToHarness`.

## D6 — Live current request never minified

**Statement:** Text under `[CURRENT REQUEST]` (the user-typed prompt) is never minified, regardless of `ZoneCurrentRequest` / `CurrentRequestMaxLevel` API knobs (retained for compatibility). Only prefixed runtime context and other zones densify.

**Rationale:** Live user ask must stay recoverable and authoritative.

**Evidence:** `minifyCurrentUserContent` hard `userBodyLevel := Off`; `TestUserLiteralPromptNeverMinified`.

## D7 — Server-config gate; product default fully off

**Statement:** Lex minify is controlled by top-level `lexMinify` config (`enabled`, `level`, `zones`, `currentRequestMaxLevel`). The entire feature is **disabled by default** (`enabled: false` → engine level 0). Operators enable via `config.yaml` or Settings UI. When `enabled: true` and `level` is left `0`, loader/settings fill `RecommendedLexMinifyLevel` (6).

**Rationale:** Feature is opt-in with safe zero defaults.

**Evidence:**

- `internal/config.LexMinifyConfig`, `EffectiveLevel`, `EngineSettings`
- Engine wiring through `app_init_services`, `chat/builders`, `cmd/agent`, delegated `SetLexMinify`
- Settings API fields `lexMinifyEnabled` / `lexMinifyLevel` / zones / current max
- UI section **Lex Minify**

**Rejected alternatives:**

- Keep package-constant hard default 6 on all product engines: rejected (user asked default feature disabled)
- Global only level without enable master: rejected — enable must be explicit

## D8 — Unminified lexminify advisory at top of system prefix

**Statement:** When `ZoneSystemPrompt` minification is active (level > 0), the first system/developer message receives a leading plain-text `[LEXMINIFY NOTICE]` block (`LexMinifyAdvisory`) that is **never** minified. The notice explains that later instructions and context may be lexically compressed, that `[CURRENT REQUEST]` stays authoritative, and that the model must not mention lexminify in responses. Additional system/developer messages are minified without a second advisory. Level 0 / zone off keeps system text and leaves the notice out.

**Rationale:** Agents need to interpret dense abbreviated instruction bodies correctly. A protected top-of-system prefix matches the product requirement that “that part should not be processed with lexminify if it is enabled.”

**Evidence:**

- `LexMinifyAdvisory` / strip+prepend in `internal/llm/lexminify/lexminify.go`
- Protect marker includes `LEXMINIFY NOTICE` in `protect.go`
- Tests: `TestMessagesZonesMinifySystemByDefault`, `TestSystemAdvisoryUnminifiedAndSingle`, `TestSystemAdvisoryAbsentWhenFeatureOff`, `TestSystemPromptZoneCanBeDisabled`

**Rejected alternatives:**

- Bake advisory into static base system prompt at compose time: rejected — would appear even when lexminify is disabled and waste tokens on non-minified runs
- Operator-docs only (no runtime notice): rejected — models would not know compressed text is intentional
- Minify the advisory too: rejected — defeats the purpose
- One advisory per system/developer message: rejected — redundant and blackmails KV budget

## Change Log (zone defaults)

1. Earlier: runtime + history (+ assistant only)
2. Then: + `ZoneSystemPrompt`
3. Then: + `ZoneTool`
4. Current: unminified `LexMinifyAdvisory` on first system/developer when system zone runs
