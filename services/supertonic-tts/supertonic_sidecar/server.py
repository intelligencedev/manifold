"""Runnable entry point: build the app from environment and serve with uvicorn.

Environment:
  SUPERTONIC_MODEL_DIR      local model dir (default: auto-download from HF)
  SUPERTONIC_AUTO_DOWNLOAD  "false" to disable HF download (default: true)
  SUPERTONIC_VOICE          default voice id (default: M1)
  SUPERTONIC_LANG           default language (default: en)
  SUPERTONIC_TOTAL_STEPS    flow-matching steps (default: 8)
  SUPERTONIC_HOST           bind host (default: 127.0.0.1)
  SUPERTONIC_PORT           bind port (default: 8890)
"""

from __future__ import annotations

import os
from typing import Mapping

from .app import create_app


def _as_bool(value: str) -> bool:
    return value.strip().lower() not in ("false", "0", "no", "off", "")


def synth_kwargs_from_env(env: Mapping[str, str]) -> dict:
    return {
        "model_dir": env.get("SUPERTONIC_MODEL_DIR") or None,
        "auto_download": _as_bool(env.get("SUPERTONIC_AUTO_DOWNLOAD", "true")),
        "default_voice": env.get("SUPERTONIC_VOICE", "M1"),
        "default_lang": env.get("SUPERTONIC_LANG", "en"),
        "total_steps": int(env.get("SUPERTONIC_TOTAL_STEPS", "8")),
    }


def build_default_app():
    from .synth import SupertonicSynth

    synth = SupertonicSynth(**synth_kwargs_from_env(os.environ))
    return create_app(synth)


def main() -> None:
    import uvicorn

    host = os.environ.get("SUPERTONIC_HOST", "127.0.0.1")
    port = int(os.environ.get("SUPERTONIC_PORT", "8890"))
    uvicorn.run(build_default_app(), host=host, port=port)


if __name__ == "__main__":
    main()
