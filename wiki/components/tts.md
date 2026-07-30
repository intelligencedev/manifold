# Component: Text-to-Speech (`/tts`)

Host-side text-to-speech for non-browser clients (and, since this session, the
Manifold Chat UI). A single `POST /tts` route fronts three interchangeable
engines behind one streaming contract, so callers never change when the backend
does.

## Reader Promise

After this page you can: call `/tts`, choose an engine via config, understand the
SSE frame contract every engine emits, know where the pure-Go `born` engine comes
from (our Born fork) and how it is wired, and understand why Manifold Chat no
longer ships onnxruntime-web.

## Key Files And Symbols

| Path | Role |
| --- | --- |
| `internal/agentd/handlers_tts.go` | `(*app).ttsHandler`, `ttsRequest{Text,Voice,Lang,SessionID}`; engine dispatch + OpenAI/sidecar proxy path |
| `internal/agentd/handlers_tts_born.go` | `(*app).ttsBornResponse`, `bornTTSHolder` (lazy one-time engine load); in-process pure-Go synthesis + SSE framing |
| `internal/agentd/router.go` | `mux.HandleFunc("/tts", a.ttsHandler())` in `registerAgentRoutes` |
| `internal/config/config.go` | `TTSConfig{Engine, ModelDir, BaseURL, Model, Voice}` |
| `internal/agentd/app.go` | `bornTTS bornTTSHolder` field on `app` |
| `services/supertonic-tts/` | Python FastAPI sidecar (engine `supertonic`) wrapping the official `supertonic` pip package |
| `services/born-supertonic-fork/` | Fork docs: `STATUS.md`, `born-changes.patch` (diff vs upstream born-ml v0.9.17) |
| `github.com/intelligencedev/born` (dep) | Pure-Go ONNX runtime + `supertonic/` pipeline package used by engine `born` |
| `web/agentd-ui/src/lib/tts/supertonic/serverEngine.ts` | Browser client: `POST /tts`, decode SSE `tts_chunk` PCM16 → Float32 |
| `web/agentd-ui/src/lib/tts/supertonic/streamer.ts` | Sentence chunking + Web Audio playback (consumes `serverEngine`) |
| `web/agentd-ui/src/lib/tts/supertonic/constants.ts` | Preset voice ids, languages, shared types (replaced the deleted `engine.ts`) |

## Engines

`tts.engine` selects the backend. The three share the client-facing contract; only
where inference runs differs.

| `tts.engine` | Where inference runs | Config used | Notes |
| --- | --- | --- | --- |
| `openai` (default) | OpenAI-compatible `/v1/audio/speech` | `baseURL`, `model`, `voice` | Original proxy path (`handlers_tts.go`) |
| `supertonic` | Python sidecar (`services/supertonic-tts`) | `baseURL` → sidecar `/v1` | Mainline `onnxruntime`; OpenAI-shaped |
| `born` | **In-process, pure Go** | `modelDir`, `voice` | No sidecar, no Python; see below |

For `openai`/`supertonic`, `handlers_tts.go` POSTs `{model,input,voice,...}` to
`${baseURL}/audio/speech`, reads the audio body, and re-frames it as SSE. For
`born`, `handlers_tts.go` short-circuits to `ttsBornResponse` (no network hop).

## Client Contract (SSE)

Every engine responds with `Content-Type: text/event-stream` and emits:

| Frame | Shape | Meaning |
| --- | --- | --- |
| audio | `{"type":"tts_chunk","bytes":N,"b64":"…","rate":44100,"seq":i}` | base64 PCM16 mono @ `rate` |
| end | `{"type":"done","rate":44100}` | stream complete |
| error | `{"type":"error","error":"…"}` (no `done`) | mid-stream failure |

The `born` path (`handlers_tts_born.go`) sub-chunks each synthesized sentence group
to **32 KB PCM per frame** so no SSE line exceeds ~43 KB of base64 — line-based
consumers (e.g. Go's `bufio.Scanner`, default 64 KB) would otherwise truncate.
Client disconnect (`r.Context().Err()`) aborts synthesis (barge-in).

## Engine `born` — in-process pure-Go Supertonic

`ttsBornResponse` loads the engine once via `bornTTSHolder` (a `sync.Once` on the
`app`) and streams `tts.SynthesizeStream(...)` output. The engine is the
`supertonic` package from our Born fork:

- Dependency: `github.com/intelligencedev/born` (go.mod pseudo-version pinned to
  fork commit `3bcdeca`). Public fork of `born-ml/born` with the module renamed so
  it is consumable as a Go dependency.
- API used: `supertonic.New(modelDir)`, `TTS.SynthesizeStream(text, voice, opts,
  emit)`, `FloatToPCM16`, `SampleRate()` (44100).
- `modelDir` must contain `onnx/` (the 4 Supertonic graphs + `tts.json` +
  `unicode_indexer.json`) and `voice_styles/` (`M1`–`M5`, `F1`–`F5`).
- Voices are presets only (`M1`–`F5`); custom Voice-Builder styles are not
  supported by this engine.

What the fork adds to make Supertonic run on Born, and its performance, are
summarized in `services/born-supertonic-fork/STATUS.md` (11 new/extended ONNX ops,
core dtype/broadcast/aliasing fixes, an im2col + register-tiled Conv1d, parallel
BatchMatMul; ~RTF 0.76 on a 16-core CPU; all four graphs numerically match
`onnxruntime` to ≤1e-5).

## Manifold Chat now uses `/tts` (not WebGPU)

Previously the Vue UI ran Supertonic in-browser via `onnxruntime-web`
(WebGPU→WASM), downloading ~380 MB of model from a CDN. This session replaced that
with `serverEngine.ts`, which streams from `/tts` and decodes PCM16 → Float32 for
Web Audio. The old in-browser engine and its dependency were removed:

- Deleted: `engine.ts`, `assets.ts`, `vendor/helper.mjs` (+ `helper.d.ts`).
- Removed `onnxruntime-web` from `package.json`/`pnpm-lock.yaml`; the built
  `dist/` no longer ships any `.wasm` / ORT chunk.
- `streamer.ts` and `stores/tts.ts` import `serverEngine as supertonicEngine`
  (playback/store code unchanged); shared constants/types moved to `constants.ts`.

The browser therefore renders with whatever `tts.engine` is configured (today
`born`) — same contract as any other `/tts` client.

## Configuration

`config.yaml` / `config.yaml.example`, `tts:` block:

```yaml
tts:
  engine: born            # openai | supertonic | born
  modelDir: /path/to/supertonic-models   # born only (onnx/ + voice_styles/)
  voice: M1               # preset for supertonic/born; OpenAI voice for openai
  baseURL: https://api.openai.com/v1   # openai/supertonic only (inert for born)
  model: gpt-4o-mini-tts               # openai/supertonic only (inert for born)
```

`baseURL`/`model` are read only by the proxy path (`handlers_tts.go`), so they are
inert when `engine: born` — kept as the ready fallback for `engine: openai`.

## Consumers

- **Manifold Chat** (`serverEngine.ts`) — browser voice output.
- **Reachy Mini bot** (separate repo) — a `ManifoldTts` engine POSTs `/tts` and
  plays the PCM on the robot speaker (resampled 44.1k→16k). See
  [TTS request flow](../flows/tts-path.md).

## Design Decisions

- **One route, three engines.** `tts.engine` swaps inference backends without
  changing the client contract; `born` needs no sidecar/Python, `supertonic`
  isolates mainline `onnxruntime` in a Python process, `openai` stays the proxy
  fallback. Evidence: `handlers_tts.go` dispatch + `TTSConfig`.
- **Fork Born rather than a Python-only sidecar for the in-process path.** A
  pure-Go engine ships in the single `agentd` binary (no Python runtime) at
  faster-than-real-time. Cost: maintaining the fork. Evidence:
  `services/born-supertonic-fork/STATUS.md`, go.mod require.
- **Move Chat TTS off client WebGPU to `/tts`.** Drops the ~380 MB client model
  download and the onnxruntime-web bundle; centralizes voice on the host. Cost:
  custom voices no longer work in-browser. Evidence: deleted `engine.ts`/`assets.ts`,
  removed dependency, `serverEngine.ts`.

## Tests / Validation

```bash
# Go route incl. live in-process born engine (needs models):
SUPERTONIC_MODEL_DIR=/path/to/models \
  go test -C manifold ./internal/agentd -run TestTTS -count=1
# Frontend client decode logic:
pnpm --dir manifold/web/agentd-ui test:unit -- --run tests/serverTtsEngine.spec.ts
```

`TestTTSBornEngineLive` drives the real handler through the fork module and
asserts a non-trivial `tts_chunk` stream + `done`. `serverTtsEngine.spec.ts`
covers SSE frame parsing across byte boundaries and PCM16→Float32 decode.
