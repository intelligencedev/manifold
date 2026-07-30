# Supertonic TTS sidecar

An HTTP service that renders [Supertonic](https://huggingface.co/Supertone/supertonic-3)
text-to-speech audio on the host, so non-browser clients (Manifold's `/tts` route, and through
it the Reachy Mini bot) can use the same voice the Manifold web UI uses.

It wraps the official `supertonic` Python package (mainline `onnxruntime`) and adds
sentence-level streaming for low time-to-first-audio.

## API

### `POST /v1/audio/speech`
OpenAI-shaped request:

```json
{ "input": "Hello there.", "voice": "M1", "lang": "en", "response_format": "pcm" }
```

- `response_format: "pcm"` (default) — streams raw **PCM16, mono, 44100 Hz** as chunked HTTP,
  one blob per sentence group. Sample rate is also returned in the `X-Sample-Rate` header.
- `response_format: "wav"` — returns a single WAV container (non-streaming; handy for debugging).
- Unknown `voice` → `400`. Missing `input` → `422`.

Voices: `M1`–`M5`, `F1`–`F5` (Supertonic presets). `model` is accepted and ignored (compat).

### `GET /healthz`
Liveness probe: `{"status": "ok", "sample_rate": 44100}`.

## Running

```bash
pip install -e .
# Use pre-downloaded models (skip the ~380MB HF download):
SUPERTONIC_MODEL_DIR=/path/to/models SUPERTONIC_AUTO_DOWNLOAD=false supertonic-sidecar
# Or let it auto-download from Hugging Face on first run:
supertonic-sidecar
```

### Environment

| Variable | Default | Purpose |
|---|---|---|
| `SUPERTONIC_MODEL_DIR` | _(auto-download)_ | Local model dir (`onnx/`, `voice_styles/`) |
| `SUPERTONIC_AUTO_DOWNLOAD` | `true` | `false` to require local models |
| `SUPERTONIC_VOICE` | `M1` | Default voice |
| `SUPERTONIC_LANG` | `en` | Default language |
| `SUPERTONIC_TOTAL_STEPS` | `8` | Flow-matching steps (lower = faster, rougher) |
| `SUPERTONIC_HOST` | `127.0.0.1` | Bind host |
| `SUPERTONIC_PORT` | `8890` | Bind port |

Point Manifold's `tts.baseURL` at `http://127.0.0.1:8890/v1` with `tts.engine: supertonic`.

## Tests

```bash
pip install -e '.[dev]'
pytest                                   # pure-logic tests always run
SUPERTONIC_TEST_MODEL_DIR=/path/to/models pytest   # also runs model-backed synth tests
```
