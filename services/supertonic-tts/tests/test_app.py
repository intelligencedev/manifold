from fastapi.testclient import TestClient

from supertonic_sidecar.app import create_app
from supertonic_sidecar.synth import UnknownVoiceError


class FakeSynth:
    sample_rate = 44100

    def stream_pcm(self, text, voice=None, lang=None):
        if voice == "bad":
            raise UnknownVoiceError("bad")
        for part in [p for p in text.split("|") if p.strip()]:
            yield part.encode()

    def synth_wav(self, text, voice=None, lang=None):
        return b"RIFF____WAVE" + b"".join(self.stream_pcm(text, voice, lang))


def client() -> TestClient:
    return TestClient(create_app(FakeSynth()))


def test_healthz_ok():
    resp = client().get("/healthz")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"


def test_speech_streams_pcm_by_default():
    resp = client().post("/v1/audio/speech", json={"input": "aa|bb|cc", "voice": "M1"})
    assert resp.status_code == 200
    assert resp.headers["content-type"].startswith("audio/pcm")
    assert resp.headers["x-sample-rate"] == "44100"
    assert resp.content == b"aabbcc"


def test_speech_wav_format_returns_container():
    resp = client().post(
        "/v1/audio/speech", json={"input": "aa|bb", "voice": "M1", "response_format": "wav"}
    )
    assert resp.status_code == 200
    assert resp.headers["content-type"].startswith("audio/wav")
    assert resp.content == b"RIFF____WAVEaabb"


def test_unknown_voice_returns_400():
    resp = client().post("/v1/audio/speech", json={"input": "hello", "voice": "bad"})
    assert resp.status_code == 400


def test_empty_input_returns_empty_stream():
    resp = client().post("/v1/audio/speech", json={"input": "   ", "voice": "M1"})
    assert resp.status_code == 200
    assert resp.content == b""


def test_missing_input_field_is_422():
    resp = client().post("/v1/audio/speech", json={"voice": "M1"})
    assert resp.status_code == 422
