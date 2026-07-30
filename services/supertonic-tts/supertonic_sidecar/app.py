"""FastAPI app exposing Supertonic synthesis over an OpenAI-shaped endpoint.

``POST /v1/audio/speech`` accepts ``{input, voice, lang, response_format}`` and
either streams raw PCM16 (``response_format="pcm"``, default) or returns a full
WAV container (``response_format="wav"``). ``GET /healthz`` is a liveness probe.
"""

from __future__ import annotations

from typing import Optional

from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse, Response, StreamingResponse
from pydantic import BaseModel

from .synth import UnknownVoiceError


class SpeechRequest(BaseModel):
    input: str
    voice: Optional[str] = None
    lang: Optional[str] = None
    response_format: str = "pcm"
    model: Optional[str] = None  # accepted for OpenAI compatibility, ignored


def create_app(synth) -> FastAPI:
    app = FastAPI(title="Supertonic TTS sidecar")
    sample_rate_header = {"X-Sample-Rate": str(synth.sample_rate)}

    @app.get("/healthz")
    def healthz():
        return JSONResponse({"status": "ok", "sample_rate": synth.sample_rate})

    @app.post("/v1/audio/speech")
    def speech(req: SpeechRequest):
        if req.response_format == "wav":
            try:
                wav = synth.synth_wav(req.input, voice=req.voice, lang=req.lang)
            except UnknownVoiceError as exc:
                raise HTTPException(status_code=400, detail=f"unknown voice: {exc}")
            return Response(content=wav, media_type="audio/wav", headers=sample_rate_header)

        # Default: stream raw PCM16. Pull the first blob eagerly so an unknown
        # voice surfaces as a 400 before the streaming response starts.
        gen = synth.stream_pcm(req.input, voice=req.voice, lang=req.lang)
        try:
            first = next(gen)
        except StopIteration:
            first = None
        except UnknownVoiceError as exc:
            raise HTTPException(status_code=400, detail=f"unknown voice: {exc}")

        def body():
            if first is not None:
                yield first
                yield from gen

        return StreamingResponse(
            body(), media_type="audio/pcm", headers=sample_rate_header
        )

    return app
