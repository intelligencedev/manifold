"""Audio encoding helpers: float32 waveform -> PCM16 and WAV container.

PCM16 conversion matches the browser reference (``vendor/helper.mjs``
``writeWavFile``): clamp to [-1, 1], scale by 32767, floor.
"""

from __future__ import annotations

import struct

import numpy as np


def float_to_pcm16(wav: np.ndarray) -> bytes:
    """Convert a float32 waveform to little-endian signed 16-bit PCM bytes."""
    flat = np.asarray(wav, dtype=np.float32).ravel()
    clamped = np.clip(flat, -1.0, 1.0)
    samples = np.floor(clamped * 32767.0).astype("<i2")
    return samples.tobytes()


def wav_bytes(pcm: bytes, sample_rate: int, channels: int = 1) -> bytes:
    """Wrap raw PCM16 bytes in a minimal WAV (RIFF) container."""
    bits_per_sample = 16
    byte_rate = sample_rate * channels * bits_per_sample // 8
    block_align = channels * bits_per_sample // 8
    data_size = len(pcm)
    header = b"".join(
        [
            b"RIFF",
            struct.pack("<I", 36 + data_size),
            b"WAVE",
            b"fmt ",
            struct.pack("<I", 16),
            struct.pack("<H", 1),  # PCM
            struct.pack("<H", channels),
            struct.pack("<I", sample_rate),
            struct.pack("<I", byte_rate),
            struct.pack("<H", block_align),
            struct.pack("<H", bits_per_sample),
            b"data",
            struct.pack("<I", data_size),
        ]
    )
    return header + pcm
