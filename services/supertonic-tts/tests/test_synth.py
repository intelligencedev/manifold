import numpy as np


def test_sample_rate_is_44100(synth):
    assert synth.sample_rate == 44100


def test_stream_pcm_yields_audio_for_a_sentence(synth):
    chunks = list(synth.stream_pcm("Hello there.", voice="M1", lang="en"))
    assert len(chunks) == 1
    pcm = chunks[0]
    assert isinstance(pcm, bytes)
    assert len(pcm) > 0
    assert len(pcm) % 2 == 0  # int16 samples
    # At least a few hundred ms of audio for a short sentence.
    samples = len(pcm) // 2
    assert samples > synth.sample_rate * 0.3


def test_stream_pcm_groups_short_sentences_into_one_chunk(synth):
    # Two short sentences fit within the 300-char group limit -> a single blob.
    chunks = list(
        synth.stream_pcm("First sentence here. Second sentence here.", voice="M1", lang="en")
    )
    assert len(chunks) == 1


def test_stream_pcm_splits_long_text_into_multiple_chunks(synth):
    # Text spanning more than one 300-char group streams as multiple blobs.
    sentence = "The quick brown fox jumps over the lazy dog every single morning. "
    long_text = sentence * 8  # ~520 chars -> at least two groups
    chunks = list(synth.stream_pcm(long_text, voice="M1", lang="en"))
    assert len(chunks) >= 2


def test_synth_wav_returns_riff_container(synth):
    wav = synth.synth_wav("Hello there.", voice="M1", lang="en")
    assert wav[0:4] == b"RIFF"
    assert wav[8:12] == b"WAVE"
    assert len(wav) > 44


def test_empty_text_yields_no_chunks(synth):
    assert list(synth.stream_pcm("   ", voice="M1", lang="en")) == []


def test_unknown_voice_raises(synth):
    import pytest

    with pytest.raises(Exception):
        list(synth.stream_pcm("Hello.", voice="does-not-exist", lang="en"))
