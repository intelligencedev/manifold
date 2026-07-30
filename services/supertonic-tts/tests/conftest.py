import os
from pathlib import Path

import pytest


def _model_dir() -> Path | None:
    raw = os.environ.get("SUPERTONIC_TEST_MODEL_DIR")
    if not raw:
        return None
    path = Path(raw)
    return path if (path / "onnx" / "vocoder.onnx").exists() else None


@pytest.fixture(scope="session")
def synth():
    """A real SupertonicSynth backed by local model files.

    Skips when SUPERTONIC_TEST_MODEL_DIR is not set to a populated model dir,
    so pure-logic tests still run in environments without the ~380MB models.
    """
    from supertonic_sidecar.synth import SupertonicSynth

    model_dir = _model_dir()
    if model_dir is None:
        pytest.skip("SUPERTONIC_TEST_MODEL_DIR not set to a populated model dir")
    return SupertonicSynth(model_dir=str(model_dir), auto_download=False)
