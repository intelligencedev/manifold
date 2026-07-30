from supertonic_sidecar.server import synth_kwargs_from_env


def test_defaults_when_env_empty():
    kw = synth_kwargs_from_env({})
    assert kw["model_dir"] is None
    assert kw["auto_download"] is True
    assert kw["default_voice"] == "M1"
    assert kw["default_lang"] == "en"
    assert kw["total_steps"] == 8


def test_reads_overrides_from_env():
    kw = synth_kwargs_from_env(
        {
            "SUPERTONIC_MODEL_DIR": "/models",
            "SUPERTONIC_AUTO_DOWNLOAD": "false",
            "SUPERTONIC_VOICE": "F2",
            "SUPERTONIC_LANG": "ko",
            "SUPERTONIC_TOTAL_STEPS": "5",
        }
    )
    assert kw["model_dir"] == "/models"
    assert kw["auto_download"] is False
    assert kw["default_voice"] == "F2"
    assert kw["default_lang"] == "ko"
    assert kw["total_steps"] == 5
