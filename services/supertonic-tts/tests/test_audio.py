import struct

import numpy as np

from supertonic_sidecar.audio import float_to_pcm16, wav_bytes


def test_float_to_pcm16_known_values():
    pcm = float_to_pcm16(np.array([0.0, 1.0, -1.0, 0.5], dtype=np.float32))
    assert struct.unpack("<4h", pcm) == (0, 32767, -32767, 16383)


def test_float_to_pcm16_clamps_out_of_range():
    pcm = float_to_pcm16(np.array([2.0, -2.0], dtype=np.float32))
    assert struct.unpack("<2h", pcm) == (32767, -32767)


def test_float_to_pcm16_length_is_two_bytes_per_sample():
    pcm = float_to_pcm16(np.zeros(100, dtype=np.float32))
    assert len(pcm) == 200


def test_wav_bytes_has_valid_riff_header():
    pcm = float_to_pcm16(np.zeros(10, dtype=np.float32))
    wav = wav_bytes(pcm, sample_rate=44100)
    assert wav[0:4] == b"RIFF"
    assert wav[8:12] == b"WAVE"
    assert struct.unpack("<I", wav[4:8])[0] == 36 + len(pcm)
    assert struct.unpack("<H", wav[22:24])[0] == 1  # mono
    assert struct.unpack("<I", wav[24:28])[0] == 44100  # sample rate
    assert struct.unpack("<H", wav[34:36])[0] == 16  # bits per sample
    assert wav[44:] == pcm
