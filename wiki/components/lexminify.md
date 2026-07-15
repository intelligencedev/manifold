# Component: lexminify

Deterministic character/word-level minification for composed LLM message contents. Pure Go: no models, no embeddings, no external deps beyond `manifold/internal/llm` message types.

## Reader Promise

After this page you can: pick a level, understand which zones minify under defaults, know what spans are protected (especially tool JSON), and flip individual zones off without rewriting the package.

## Key Files And Symbols

| Path | Role |
| --- | --- |
| `internal/llm/lexminify/lexminify.go` | Levels, zone bitmask, `MinifyString`, `MinifyMessages`, progressive transforms |
| `internal/llm/lexminify/protect.go` | Protected-span scanner (code, URLs, UUIDs, paths, JSON/YAML-ish blobs, section markers) |
| `internal/llm/lexminify/tables.go` | Filler phrases, stopwords, telegram drops, abbreviations, **protected polarity words** |
| `internal/llm/lexminify/lexminify_test.go` | Level, protection, zone default/disable tests |
| `internal/agent/engine_lexminify.go` | `(*Engine).lexMinifyForProvider` adapter + `lexminify_applied` log |
| `internal/agent/engine.go` | `DefaultLexMinifyLevel`, `LexMinifyLevel`, `LexMinifyZones`, `LexMinifyCurrentMax` |

## Levels

Progressive (higher implies lower). Public constants in `lexminify.go`:

| Engine / Options value | Constant | Behavior |
| ---: | --- | --- |
| 0 | `Off` | Disabled |
| 1 | `L0Whitespace` | Collapse whitespace / blank lines |
| 2 | `L1Fillers` | Low-value filler phrase remove/substitute |
| 3 | `L2Stopwords` | Careful stopword drop; never polarity-protected words |
| 4 | `L3Telegram` | Function-word drop + abbreviations |
| 5 | `L4Vowels` | Vowel skeleton on long words (len ≥ 5) |
| 6 | `L5Aggressive` | Densify residual punctuation/spacing on non-protected prose |

Product default: `agent.DefaultLexMinifyLevel = 6` maps to `L5Aggressive`. Values above 6 clamp to 6.

## Zones

`Zone` is a bitmask. `Options.Zones == 0` (and engine `LexMinifyZones == 0`) means **`DefaultZones`**.

```go
const DefaultZones = ZoneRuntimeContext | ZoneHistory | ZoneTool | ZoneAssistant | ZoneSystemPrompt
```

| Zone | Targets | In DefaultZones? |
| --- | --- | --- |
| `ZoneRuntimeContext` | `[RUNTIME CONTEXT]` blocks on the current user message | yes |
| `ZoneHistory` | Non-current user msgs; participates with assistant zone on history | yes |
| `ZoneCurrentRequest` | `[CURRENT REQUEST]` body | **no** (enable deliberately) |
| `ZoneTool` | `role == "tool"` payloads | **yes** |
| `ZoneAssistant` | Assistant-role historical content | yes |
| `ZoneSystemPrompt` | Leading system/developer prefix | yes |

### ZoneTool (working change)

- Tool-role branch in `MinifyMessages` already existed; the working product change is **flipping the default bit on**.
- Same top-level `Level` applies; no `LexMinifyToolMax` cap.
- Structured tool output stays intact via `protect.go` (fenced/inline code, balanced `{…}`/`[…]` heuristics, URLs, UUIDs, paths, numbers+units, section markers).
- Tests: `TestMessagesZonesMinifyToolByDefault`, `TestToolZoneCanBeDisabled`.

### ZoneSystemPrompt

- Minifies system/developer prefix when bit set.
- Drop bit to keep a stable KV-cache-friendly prefix:
  `ZoneRuntimeContext | ZoneHistory | ZoneTool | ZoneAssistant`.

### Current request

- Marker-aware split in `minifyCurrentUserContent`.
- Off unless `ZoneCurrentRequest` set; then default cap is `L0Whitespace` unless `CurrentRequestMaxLevel` / `LexMinifyCurrentMax` raises it (never above global level).

## Protection Invariants

Applied inside `minifyUnprotected` before transforms:

- Fenced code ```…```
- Inline code `` `…` ``
- URLs / emails
- UUIDs
- File paths (unix-ish / windows-ish)
- Numbers (+ common units)
- Balanced JSON-ish objects/arrays (length ≥ 16, colon/quote heuristic)
- Consecutive YAML-looking key lines (≥ 3)
- Section markers: `[RUNTIME CONTEXT]`, `[CURRENT REQUEST]`, `[CONVERSATION HISTORY]`

Polarity/negation words never dropped as stopwords (tables): e.g. `not`, `no`, `never`, `without`/`w/o`, `except`, `only`, `must`, `cannot`, and several contractions/related guards.

## API Surface

```go
func MinifyString(s string, level int) string
func MinifyMessages(msgs []llm.Message, opts Options) Result

type Options struct {
    Level                  int
    Zones                  Zone // 0 => DefaultZones
    CurrentRequestMaxLevel int
}

type Result struct {
    Messages        []llm.Message // shallow content copy when changed
    Level, Zones    // effective
    Changed         bool
    MessagesTouched int
    RunesBefore, RunesAfter int
}
```

Invariants:

- Input message slice is not mutated (copy-out).
- Role routing only rewrites `Content` (tool IDs / other fields preserved).

## Engine Configuration

| Field | Meaning |
| --- | --- |
| `Engine.LexMinifyLevel` | 0 off; product constructions set `DefaultLexMinifyLevel` (6) |
| `Engine.LexMinifyZones` | Bitmask of `lexminify.Zone`; 0 → package `DefaultZones` |
| `Engine.LexMinifyCurrentMax` | Cap for current-request zone when enabled |

Construction sites setting level 6:

- `internal/agentd/app_init_services.go` (`initEngine`)
- `internal/agentd/chat/builders.go` (`BuildSpecialist`, `BuildTeam`)
- `internal/tools/agents/delegator.go`
- `internal/tools/agents/agent_call.go`
- `cmd/agent/main.go` (`runOrchestrator`)

Bare `Engine{}` in tests remains level 0 (off) unless set — intentional so unit tests without minification stay quiet.

## How To Disable / Narrow

| Goal | Knob |
| --- | --- |
| Off entirely | `eng.LexMinifyLevel = 0` |
| Skip tool results only | set `LexMinifyZones` to `DefaultZones` **without** `ZoneTool` |
| Skip system prefix only | omit `ZoneSystemPrompt` |
| Enable current-request minify lightly | OR in `ZoneCurrentRequest`; optionally set `LexMinifyCurrentMax` |
| Force non-default bitmask | any non-zero `LexMinifyZones` replaces `DefaultZones` entirely (compose the full mask you want) |

Example — disable only tool zone while keeping other defaults:

```go
eng.LexMinifyZones = int(
    lexminify.ZoneRuntimeContext |
    lexminify.ZoneHistory |
    lexminify.ZoneAssistant |
    lexminify.ZoneSystemPrompt,
)
```

## Observability

When minification changes any message content, adapter logs:

- event: `lexminify_applied`
- fields: `level`, `zones`, `messages_touched`, `runes_before`, `runes_after`

## Tests And Validation

```bash
go test -C manifold ./internal/llm/lexminify ./internal/agent -count=1
```

Notable package tests:

- Progressive level + protection: string-level cases in `lexminify_test.go`
- `TestMessagesZonesMinifySystemByDefault` / `TestSystemPromptZoneCanBeDisabled`
- `TestMessagesZonesMinifyToolByDefault` / `TestToolZoneCanBeDisabled`
- Determinism check: same input ⇒ same output

## Evidence

- Package header + constants: `internal/llm/lexminify/lexminify.go`
- Protection scanners: `internal/llm/lexminify/protect.go`
- Protected polarity map: `internal/llm/lexminify/tables.go` (`protectedWords`)
- Tool and system zone tests: `internal/llm/lexminify/lexminify_test.go`
- Adapter: `internal/agent/engine_lexminify.go`

## Unknowns / Risks

- Heuristic JSON/YAML detection is best-effort (`≥16` chars, colon/quote counts); exotic tool dumps may partially transform prose wrappers only, which is intended, but **unusual formats should be fixture-tested before tightening defaults again**.
- Tokenizer savings ≠ rune savings; measure on production-like payloads before claiming %.