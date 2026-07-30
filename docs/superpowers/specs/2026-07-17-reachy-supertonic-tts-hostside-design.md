# Host-side Supertonic TTS for Reachy Mini (Path A)

**Date:** 2026-07-17
**Status:** Approved design, pending implementation plan
**Repos:** `manifold` (Go backend + Vue frontend), `reachy_mini_manifold_assistant` ("Minibot", Python Reachy Mini app)

## Summary

Give the Reachy Mini bot a second TTS option alongside the OpenAI Realtime API: Manifold's
Supertonic voice, rendered **host-side** and streamed to the robot as audio. The robot becomes a
speaker for Supertonic; it does not run the model itself.

## Goals

- Reachy Mini can speak using Supertonic (`Supertone/supertonic-3`), the same model Manifold's
  browser UI uses, selected via `REACHY_TTS_ENGINE=manifold`.
- Low time-to-first-audio via sentence-level streaming.
- Manifold owns TTS configuration (engine, voice); the robot talks to one backend.
- A stable `/tts` contract so the inference backend can change later without touching the robot.

## Non-goals

- Replacing the OpenAI Realtime API wholesale. Supertonic is TTS-only; STT + LLM turns still come
  from Manifold's existing pipeline. This adds a voice-output option, not a full duplex stack.
- Running Supertonic on the robot's Pi 5 (that was Path B, deferred).
- In-process Go inference (see "Rejected alternatives").

## Background & key findings

Supertonic currently runs **only in the browser** (`web/agentd-ui/src/lib/tts/supertonic/`) via
`onnxruntime-web` (WebGPU→WASM). It is four ONNX graphs — `duration_predictor`, `text_encoder`,
`vector_estimator`, `vocoder` (opset 19, ~380 MB total) — plus pipeline glue in
`vendor/helper.mjs` (`UnicodeProcessor` tokenization, a 5-step flow-matching sampler, style-tensor
injection, WAV assembly). **The pipeline glue is not in the `.onnx` files**, so any host port must
reimplement it (or reuse Supertone's Python reference).

The Manifold Go backend (`agentd`) has **zero server-side ML inference**. Existing TTS/STT paths
proxy to an external OpenAI-compatible endpoint (`internal/tools/tts/tool.go`, `TTSConfig`).

**Model facts (verified from `tts.json`):** output sample rate **44100 Hz mono**. The Reachy
speaker is native **16000 Hz float32** (`push_audio_sample`), so the robot downsamples 44.1k→16k.
Supertonic runs ~RTF 0.3 (3× real-time) on modest ARM CPU, so CPU inference is ample; GPU is
optional.

### Rejected alternative: pure-Go / in-process inference

A feasibility spike (2026-07-17) tested `born-ml/born` v0.9.17 (pure-Go, zero-CGO ONNX runtime).
**Result: cannot run Supertonic.** Born implements 57 (transformer/LLM-shaped) ops; Supertonic
needs 8 that Born lacks: `Pad, BatchNormalization, Cos, Sin, Reciprocal, ReduceSum, Softplus,
Tile`. `Pad` alone appears in all four graphs (wraps every convolution). Strict-mode load fails on
all four graphs; non-strict mode silently skips unsupported ops → garbage audio with no error.
Other pure-Go/no-CGO avenues also fail: `onnxruntime-purego` still ships a native lib and is
experimental; `gonnx`/`onnx-go` are op-starved; `gogpu/wgpu` is a GPU-primitive layer, not an
inference engine; WONNX is Rust with the same op-coverage risk. GPU acceleration is moot given
Supertonic's CPU speed.

Decision: use mainline `onnxruntime` in a **Python sidecar**. The pure-Go single-binary path stays
alive but decoupled — implementing the 8 ops upstream in Born is a standalone future track gated
only by the `/tts` contract.

## Architecture

Three isolated, independently testable units joined by one stable seam — Manifold `POST /tts`.

```
User text → Minibot → Manifold /agent/run (chat, existing)
                          └→ assistant reply {speak, tools}
Minibot ManifoldTts.speak(text)
   → Manifold POST /tts  (SSE tts_chunk frames)
        → Supertonic sidecar POST /v1/audio/speech  (streams PCM per sentence)
   ← decode b64 PCM16 @44.1k → resample→16k → float32 → media.push_audio_sample()
```

TTS is a **separate call** from `/agent/run`, mirroring the browser (chat text and Supertonic
synthesis are decoupled). The robot gets reply text from chat, then requests audio.

### Unit 1 — Supertonic sidecar (new Python service)

- Loads the 4 ONNX graphs via mainline `onnxruntime` (CPU EP default; CUDA/CoreML optional) plus
  the reference pipeline (port of `helper.mjs`, or Supertone's Python reference if available).
- **Endpoint:** `POST /v1/audio/speech` — OpenAI-shaped request
  `{model, input, voice, response_format:"pcm", stream:true}` so it slots into Manifold's existing
  `TTSConfig.baseURL` proxy pattern. Also `GET /healthz`.
- **Streaming:** splits `input` into sentences, synthesizes each, flushes its PCM16 as it
  completes (mirrors the frontend `SupertonicStreamer`). This drives low time-to-first-audio.
- **Response:** raw PCM16 mono @44100 Hz, chunked; sample rate + voice conveyed in headers (or a
  leading JSON metadata frame).
- **Voice:** maps `voice` → preset styles (`M1`…`F5`) / custom voice styles.
- **Config:** model dir (local path or HF download+cache), host/port, device, default voice.

### Unit 2 — Manifold `/tts` route (Go, new)

- `POST /tts` registered in `registerAgentRoutes` (`internal/agentd/router.go`), handler in
  `handlers_media.go` mirroring `sttHandler`. Request `{text, voice?, sessionId?}`.
- When `tts.engine == "supertonic"`, proxies to the sidecar (`tts.baseURL`) and **re-emits the
  existing SSE `tts_chunk` frames** (`{type:"tts_chunk", b64, rate, seq}`) plus a terminal
  `tts_audio`/`done`. Reuses streaming/WAV-assembly helpers in `internal/tools/tts`.
- **Config:** extend `TTSConfig` (`internal/config/config.go`) with `engine: openai|supertonic`,
  reusing `baseURL`/`voice`. Default sidecar URL e.g. `http://127.0.0.1:8890/v1`.
- **Auth:** identical posture to sibling agent routes (open when `Auth.Enabled` is false, the
  typical local deploy).

### Unit 3 — Minibot `ManifoldTts` engine (Python, `speech.py`)

- Claims the `REACHY_TTS_ENGINE=manifold` stub (currently a warning fallback, `speech.py:210-217`).
- `speak(text)`: POST to Manifold `/tts` (httpx streaming), parse `tts_chunk` frames,
  base64-decode PCM16, resample 44.1k→16k (`resample_int16`, `openai_realtime.py:31-65`), convert
  to float32 (`audio_to_float32`), call `media.push_audio_sample()` after `media.start_playing()`.
- `stop()`: abort the stream and clear the player (barge-in / new turn).
- **Fallback:** on error/timeout, degrade to the existing local `RobotSoundTts` so the robot still
  speaks. Wired via a `make_tts` factory branch (`speech.py:197-219`).

## Data flow (end to end)

1. User text enters Minibot (console loop or `POST /chat` settings route).
2. Minibot → Manifold `POST /agent/run` → assistant reply parsed to `{speak, tools}`
   (`protocol.py`).
3. `on_speak(text)` → `ManifoldTts.speak(text)`.
4. Minibot → Manifold `POST /tts` (streaming).
5. Manifold → sidecar `POST /v1/audio/speech` (streaming); relays PCM as SSE `tts_chunk`.
6. Minibot decodes each frame, resamples 44.1k→16k, pushes float32 to the speaker.
7. Robot tools (if any) execute via the existing `ToolExecutor` in parallel/after, unchanged.

## Configuration

- **Manifold** (`config.yaml`): `tts.engine: supertonic`, `tts.baseURL: <sidecar>/v1`, `tts.voice`.
- **Minibot** (`.env`): `REACHY_TTS_ENGINE=manifold`, existing `MANIFOLD_BASE_URL`, optional voice.
- **Sidecar**: model dir, port, device, default voice (env or flags).

## Error handling

- Sidecar unreachable → `/tts` returns 502 → Minibot falls back to local `RobotSoundTts`.
- Empty/failed synthesis for a sentence → skipped; remaining sentences continue.
- New user turn while speaking → Minibot aborts the HTTP stream and clears the player (barge-in).
- Sidecar model-load failure → `/healthz` unhealthy; Manifold surfaces a clear error.

## Testing

- **Sidecar (pytest):** sentence-split; synth returns non-empty PCM @44100 for a sentence; SSE /
  chunk framing; voice mapping; optional audio-parity smoke vs. the browser reference output.
- **Manifold (Go):** `/tts` handler test with a mock sidecar — chunk relay, `engine` selection,
  error→502; config parsing for `engine`.
- **Minibot (pytest):** `ManifoldTts` with mock media + mock stream — frame parse, `resample_int16`
  invoked, `push_audio_sample` invoked, fallback to local TTS on error.
- **Manual E2E:** robot speaks a streamed multi-sentence reply; verify first audio arrives before
  full synthesis completes.

## Deployment

Sidecar runs alongside Manifold on the host (process/systemd/container). Models (~380 MB)
downloaded once by the sidecar (HF or a local path). Manifold config points `tts.baseURL` at it.

## Interface isolation / future-proofing

The `/tts` contract (request `{text, voice?}`, response SSE `tts_chunk`) is the stable seam. The
sidecar is language-isolated and swappable: a future in-process Go implementation (Born + the 8
missing ops, once contributed) can replace it behind the same route without changing Manifold's
handler contract or the Minibot engine.

## Open questions (non-blocking)

- Whether the sidecar should also expose a non-streaming WAV endpoint for debugging (proposed: yes,
  cheap to add).
- Whether voice selection is robot-driven (per request) or fixed in Manifold config (design
  supports both; default to Manifold config with optional per-request override).
