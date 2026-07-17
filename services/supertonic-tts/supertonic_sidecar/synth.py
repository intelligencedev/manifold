"""Supertonic synthesis engine wrapper.

Thin layer over the official ``supertonic`` package that adds sentence-level
streaming: text is split into sentence groups and each group is synthesized and
emitted as PCM16 as soon as it is ready, minimizing time-to-first-audio.
"""

from __future__ import annotations

from typing import Iterator, Optional

import numpy as np
from supertonic import TTS

from .audio import float_to_pcm16, wav_bytes
from .text import split_sentences


class UnknownVoiceError(Exception):
    """Raised when a requested voice cannot be resolved."""


class SupertonicSynth:
    def __init__(
        self,
        model: str = "supertonic-3",
        model_dir: Optional[str] = None,
        auto_download: bool = True,
        default_voice: str = "M1",
        default_lang: str = "en",
        total_steps: int = 8,
        speed: float = 1.05,
        silence_duration: float = 0.3,
    ) -> None:
        self._tts = TTS(model=model, model_dir=model_dir, auto_download=auto_download)
        self.sample_rate: int = int(self._tts.sample_rate)
        self.default_voice = default_voice
        self.default_lang = default_lang
        self.total_steps = total_steps
        self.speed = speed
        self.silence_duration = silence_duration
        self._styles: dict = {}

    def _style(self, voice: str):
        if voice not in self._styles:
            try:
                self._styles[voice] = self._tts.get_voice_style(voice)
            except Exception as exc:  # missing style file, bad name, etc.
                raise UnknownVoiceError(voice) from exc
        return self._styles[voice]

    @staticmethod
    def _max_chunk_len(lang: str) -> int:
        return 120 if lang in ("ko", "ja") else 300

    def stream_pcm(
        self,
        text: str,
        voice: Optional[str] = None,
        lang: Optional[str] = None,
    ) -> Iterator[bytes]:
        """Yield PCM16 (mono, ``sample_rate`` Hz) bytes, one blob per sentence group."""
        voice = voice or self.default_voice
        lang = lang or self.default_lang
        style = self._style(voice)  # raises for an unknown voice
        for sentence in split_sentences(text, self._max_chunk_len(lang)):
            wav, _duration = self._tts.synthesize(
                sentence,
                voice_style=style,
                total_steps=self.total_steps,
                speed=self.speed,
                silence_duration=self.silence_duration,
                lang=lang,
            )
            pcm = float_to_pcm16(np.asarray(wav))
            if pcm:
                yield pcm

    def synth_wav(
        self,
        text: str,
        voice: Optional[str] = None,
        lang: Optional[str] = None,
    ) -> bytes:
        """Synthesize the whole text and return a single WAV container (debug/non-streaming)."""
        pcm = b"".join(self.stream_pcm(text, voice, lang))
        return wav_bytes(pcm, self.sample_rate)
