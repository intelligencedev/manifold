# Component: lexminify

Deterministic character/word-level minification for composed LLM message contents. Pure Go: no models, no embeddings, no external deps beyond `manifold/internal/llm` message types.

## Reader Promise

After this page you can: pick a level, understand which zones minify under defaults, know what spans are protected (especially tool JSON), understand the unminified system advisory, and flip individual zones off without rewriting the package.

## Key Files And Symbols

| Path | Role |
| --- | --- |
| `internal/llm/lexminify/lexminify.go` | Levels, zone bitmask, `MinifyString`, `MinifyMessages`, `LexMinifyAdvisory`, progressive transforms |
| `internal/llm/lexminify/protect.go` | Protected-span scanner (code, URLs, UUIDs, paths, JSON/YAML-ish blobs, section markers) |
| `internal/llm/lexminify/tables.go` | Filler phrases, stopwords, telegram drops, abbreviations, **protected polarity words** |
| `internal/llm/lexminify/lexminify_test.go` | Level, protection, zone default/disable, advisory tests |
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

Product default recommendation: `agent.DefaultLexMinifyLevel = 6` maps to `L5Aggressive` when the feature is enabled via config. Values above 6 clamp to 6. Config default is `enabled: false` (see D7).

## Zones

`Zone` is a bitmask. `Options.Zones == 0` (and engine `LexMinifyZones == 0`) means **`DefaultZones`**.

```go
const DefaultZones = ZoneRuntimeContext | ZoneHistory | ZoneTool | ZoneAssistant | ZoneSystemPrompt
```

| Zone | Targets | In DefaultZones? |
| --- | --- | --- |
| `ZoneRuntimeContext` | `[RUNTIME CONTEXT]` blocks on the current user message | yes |
| `ZoneHistory` | Non-current user msgs; participates with assistant zone on history | yes |
| `ZoneCurrentRequest` | `[CURRENT REQUEST]` body (API retained; live body never minified) | **no** |
| `ZoneTool` | `role == "tool"` payloads | **yes** |
| `ZoneAssistant` | Assistant-role historical content | yes |
| `ZoneSystemPrompt` | Leading system/developer prefix | yes |

### ZoneTool

- Tool-role branch in `MinifyMessages` minifies NL wrappers; structured JSON/YAML/URLs stay intact via `protect.go`.
- Same top-level `Level` applies; no `LexMinifyToolMax` cap.
- Tests: `TestMessagesZonesMinifyToolByDefault`, `TestToolZoneCanBeDisabled`.

### ZoneSystemPrompt + LexMinifyAdvisory

- Minifies system/developer prefix when bit set.
- When active (and level > 0), the **first** system/developer message is rewritten so an unminified plain-text advisory (`[LEXMINIFY NOTICE]` / `LexMinifyAdvisory`) sits **above** the minified instruction body.
- The advisory itself is **never** minified. Minify strips any prior notice first, then re-attaches the constant, so notices never stack on re-minify.
- Later system/developer prefix messages are minified without a second advisory.
- Drop the bit to keep a stable KV-cache-friendly prefix with no advisory:
  `ZoneRuntimeContext | ZoneHistory | ZoneTool | ZoneAssistant`.
- Tests: `TestMessagesZonesMinifySystemByDefault`, `TestSystemAdvisoryUnminifiedAndSingle`, `TestSystemAdvisoryAbsentWhenFeatureOff`, `TestSystemPromptZoneCanBeDisabled`.

### Current request

- Marker-aware split in `minifyCurrentUserContent`.
- The live user body under `[CURRENT REQUEST]` is **never** minified (hard invariant), regardless of `ZoneCurrentRequest` / `CurrentRequestMaxLevel`.

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
- Section markers: `[RUNTIME CONTEXT]`, `[CURRENT REQUEST]`, `[CONVERSATION HISTORY]`, `[LEXMINIFY NOTICE]`

Polarity/negation words never dropped as stopwords (tables): e.g. `not`, `no`, `never`, `without`/`w/o`, `except`, `only`, `must`, `cannot`, and several contractions/related guards.

## API Surface

```go
func MinifyString(s string, level int) string
func MinifyMessages(msgs []llm.Message, opts Options) Result

const LexMinifyAdvisoryMarker = "[LEXMINIFY NOTICE]"
const LexMinifyAdvisory = LexMinifyAdvisoryMarker + "\n…plain text…\n---"

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
- Advisory is provider-visible only (via the same copy path); permanent history is untouched.

## Engine Configuration

| Field | Meaning |
| --- | --- |
| `Engine.LexMinifyLevel` | 0 off; product constructions use `config.LexMinify.EngineSettings()` |
| `Engine.LexMinifyZones` | Bitmask of `lexminify.Zone`; 0 → package `DefaultZones` |
| `Engine.LexMinifyCurrentMax` | Retained API cap; live `[CURRENT REQUEST]` body still never minified |

Bare `Engine{}` in tests remains level 0 (off) unless set — intentional so unit tests without minification stay quiet.

## How To Disable / Narrow

| Goal | Knob |
| --- | --- |
| Off entirely | `eng.LexMinifyLevel = 0` (or `lexMinify.enabled: false`) |
| Skip tool results only | set `LexMinifyZones` to `DefaultZones` **without** `ZoneTool` |
| Skip system prefix (+ advisory) only | omit `ZoneSystemPrompt` |
| Force non-default bitmask | any non-zero `LexMinifyZones` replaces `DefaultZones` entirely |

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
- `TestSystemAdvisoryUnminifiedAndSingle` / `TestSystemAdvisoryAbsentWhenFeatureOff`
- `TestMessagesZonesMinifyToolByDefault` / `TestToolZoneCanBeDisabled`
- `TestUserLiteralPromptNeverMinified`
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
- Advisory adds a modest fixed token cost when system-zone minify is on; not injected when the feature is off.
