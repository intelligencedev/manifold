# Evidence: lexminify Working State

## Commands Run

```bash
go test -C manifold ./internal/llm/lexminify ./internal/agent -count=1
# Reported: PASS (prior session)
```

## Source Anchors

| Claim | Where |
| --- | --- |
| Package purpose + level ladder L0–L5 with values 1–6 | `internal/llm/lexminify/lexminify.go` package comment + consts |
| `DefaultZones` includes tool + system | same file, `const DefaultZones = …` |
| Tool role minify gate | `MinifyMessages` `case role == "tool"` |
| System/developer gate | `case i < prefixEnd, role == "system", role == "developer"` |
| Protected span kinds | `internal/llm/lexminify/protect.go` (`reFenced`, URLs, UUID, path, structured blobs, markers) |
| Never-drop polarity set | `internal/llm/lexminify/tables.go` `protectedWords` |
| Engine default level 6 | `internal/agent/engine.go` `DefaultLexMinifyLevel = 6` |
| Engine zone zero comment includes tool | `Engine.LexMinifyZones` field comment |
| Adapter + log event | `internal/agent/engine_lexminify.go` (`lexminify_applied`) |
| Loop hooks | `internal/agent/engine_loop.go` `providerMsgs := e.lexMinifyForProvider(...)` (chat + stream) |
| Harness hook | `internal/agent/engine_harness.go` `providerSerial := e.lexMinifyForProvider(...)` |
| Default level assignment sites | `app_init_services.go`, `chat/builders.go`, `delegator.go`, `agent_call.go`, `cmd/agent/main.go` |
| ZoneTool default tests | `TestMessagesZonesMinifyToolByDefault`, `TestToolZoneCanBeDisabled` |
| ZoneSystem default tests | `TestMessagesZonesMinifySystemByDefault`, `TestSystemPromptZoneCanBeDisabled` |

## Inference (labeled)

- Protection heuristics are “good enough for common tool JSON + prose wrappers” based on tests, not exhaustive production corpus audit.
- Aggressive L5 on tool natural-language side is acceptable because structured islands are span-protected; residual risk is monolothic free-text tools without markers.

## Unknowns

- Net **token** reduction vs rune reduction on OpenAI/Anthropic tokenizers not measured in this wiki pass
- Whether production tool payloads routinely exceed heuristic structured-blob thresholds (16 chars, colon counts)

---

# Evidence: TTS (`/tts`) Working State

## Commands Run

```bash
# Go route incl. live in-process born engine (fork module):
SUPERTONIC_MODEL_DIR=~/.cache/manifold/supertonic-models \
  go test -C manifold ./internal/agentd -run TestTTS -count=1
# Reported: TestTTSBornEngineLive PASS — "born in-process: 11 chunks, 3.72s audio"

# Frontend client decode:
pnpm --dir manifold/web/agentd-ui test:unit -- --run tests/serverTtsEngine.spec.ts
# Reported: 4 passed
```

## Source Anchors

| Claim | Where |
| --- | --- |
| `/tts` route registration | `internal/agentd/router.go` `mux.HandleFunc("/tts", a.ttsHandler())` |
| Engine dispatch (born short-circuit vs proxy) | `internal/agentd/handlers_tts.go` `ttsHandler`, `strings.EqualFold(...,"born")` |
| Proxy path reads baseURL/model | `handlers_tts.go` `a.cfg.TTS.BaseURL`, `a.cfg.TTS.Model` |
| In-process engine + lazy load | `internal/agentd/handlers_tts_born.go` `ttsBornResponse`, `bornTTSHolder` (`sync.Once`) |
| SSE frames `tts_chunk`/`done`/`error`, 32 KB sub-chunk | `handlers_tts_born.go` `writeFrame`, `frameBytes = 32 * 1024` |
| Barge-in / disconnect abort | `handlers_tts_born.go` `r.Context().Err()` in `emit` |
| Config surface | `internal/config/config.go` `TTSConfig{Engine, ModelDir, BaseURL, Model, Voice}` |
| `bornTTS` holder on app | `internal/agentd/app.go` |
| Fork dependency (pinned) | `go.mod` `github.com/intelligencedev/born v0.0.0-20260718015058-3bcdeca4753c` |
| Fork changes + perf | `services/born-supertonic-fork/STATUS.md`, `born-changes.patch` |
| Python sidecar (engine `supertonic`) | `services/supertonic-tts/` (FastAPI + official `supertonic` pip pkg) |
| Browser client decode | `web/agentd-ui/src/lib/tts/supertonic/serverEngine.ts` |
| Chat playback consumes serverEngine | `web/agentd-ui/src/lib/tts/supertonic/streamer.ts`, `stores/tts.ts` (`serverEngine as supertonicEngine`) |
| WebGPU engine removed | deleted `engine.ts`/`assets.ts`/`vendor/helper.mjs`; `onnxruntime-web` dropped from `web/agentd-ui/package.json` + `pnpm-lock.yaml` |

## Inference (labeled)

- RTF ~0.76 and ≤1e-5 graph parity are from this session's benchmarks on a 16-core
  Apple-silicon CPU (see `STATUS.md`); other hardware (e.g. Raspberry-Pi-class)
  is expected slower and was not measured.
- The `born` engine load cost is amortized after the first request (`sync.Once`);
  first-request latency includes loading the 4 ONNX graphs.

## Unknowns

- Concurrency behavior of a single shared `supertonic.TTS` under many simultaneous
  `/tts` requests (the fork parallelizes within one synth; cross-request locking
  not audited here).
- Whether preset voice coverage (`M1`–`F5`) is sufficient for product needs; custom
  Voice-Builder styles are unsupported by the `born` engine.