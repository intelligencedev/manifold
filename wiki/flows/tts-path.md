# Flow: `/tts` Request → Streamed Audio

## Purpose

Show how a text-to-speech request moves through the `/tts` route to a streamed
`tts_chunk` response, for each engine, and how the two consumers (Manifold Chat,
the Reachy Mini bot) play it. Related: [TTS component](../components/tts.md).

## Dispatch

```mermaid
flowchart TD
  Req["POST /tts {text, voice?, lang?}"] --> H["ttsHandler (handlers_tts.go)"]
  H -->|"engine == born"| Born["ttsBornResponse (in-process)"]
  H -->|"engine == openai | supertonic"| Proxy["proxy → ${baseURL}/audio/speech"]
  Born --> SSE["SSE: tts_chunk* + done"]
  Proxy --> SSE
  SSE --> Browser["Manifold Chat serverEngine.ts"]
  SSE --> Robot["Reachy Mini ManifoldTts"]
```

Evidence: `internal/agentd/handlers_tts.go` (engine branch), `handlers_tts_born.go`
(born path), `router.go` (`/tts` registration).

## In-process `born` sequence

```mermaid
sequenceDiagram
  participant C as Client (browser / robot)
  participant H as ttsHandler
  participant B as ttsBornResponse
  participant E as supertonic.TTS (fork)
  C->>H: POST /tts {text, voice}
  H->>H: engine == "born"
  H->>B: ttsBornResponse(w, r, req)
  B->>B: bornTTS.get(modelDir)  (sync.Once load)
  B->>E: SynthesizeStream(text, voice, opts, emit)
  loop per sentence group
    E-->>B: wav []float32
    B->>B: FloatToPCM16 → 32KB sub-chunks
    B-->>C: data: {type:tts_chunk, b64, rate:44100, seq}
    Note over B,C: r.Context().Err() aborts (barge-in)
  end
  B-->>C: data: {type:done, rate:44100}
```

`bornTTSHolder` caches the loaded engine (and any load error) so only the first
request pays the ~4-graph ONNX load. Evidence: `handlers_tts_born.go`.

## Consumer playback

```mermaid
flowchart LR
  subgraph Browser["Manifold Chat"]
    S1["serverEngine.synthesize()"] --> D1["b64 → Int16 → Float32"]
    D1 --> WA["Web Audio (44.1k, auto-resampled)"]
  end
  subgraph Robot["Reachy Mini"]
    S2["ManifoldTts.speak()"] --> D2["b64 → Int16 PCM"]
    D2 --> RS["resample 44.1k → 16k"]
    RS --> PUSH["media.push_audio_sample (float32)"]
  end
```

- **Browser** (`serverEngine.ts`): buffers `tts_chunk` frames into one
  `Float32Array` per synthesized chunk and hands it to the existing
  `SupertonicStreamer` playback (Web Audio resamples 44.1 kHz to the context rate).
- **Robot** (separate repo, `speech.py` `ManifoldTts`): decodes PCM16, resamples
  44.1 kHz → the Reachy speaker's 16 kHz, and pushes float32 frames. Streaming per
  sentence group keeps time-to-first-audio low.

## Errors and edge cases

- `engine: born` but no `modelDir` → `503` before any frame.
- Engine load failure (bad model dir) → `503` (cached; not retried per request).
- Mid-stream synthesis error → in-band `{"type":"error"}` frame, `done` withheld,
  so a truncated stream is not mistaken for success.
- Empty `text` → empty stream (no `tts_chunk`).
- Client disconnect → synthesis stops (checked each emitted group).

Evidence: `handlers_tts_born.go` (status codes, error frame, context check);
`web/agentd-ui/src/lib/tts/supertonic/serverEngine.ts` (client-side empty/HTTP-error
handling).
