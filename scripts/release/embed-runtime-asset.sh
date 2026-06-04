#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

RUNTIME_ARCHIVE=""
RUNTIME_MANIFEST=""
TARGET_OS=""
TARGET_ARCH=""

usage() {
  cat <<'USAGE'
Usage: embed-runtime-asset.sh --runtime-archive <path> --runtime-manifest <path> \
  --target-os <os> --target-arch <arch>
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --runtime-archive) RUNTIME_ARCHIVE="$2"; shift 2 ;;
    --runtime-manifest) RUNTIME_MANIFEST="$2"; shift 2 ;;
    --target-os) TARGET_OS="$2"; shift 2 ;;
    --target-arch) TARGET_ARCH="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

[[ -f "$RUNTIME_ARCHIVE" ]] || { echo "Runtime archive not found: $RUNTIME_ARCHIVE" >&2; exit 2; }
[[ -f "$RUNTIME_MANIFEST" ]] || { echo "Runtime manifest not found: $RUNTIME_MANIFEST" >&2; exit 2; }
[[ -n "$TARGET_OS" && -n "$TARGET_ARCH" ]] || { usage; exit 2; }

export RUNTIME_MANIFEST TARGET_OS TARGET_ARCH
python3 - <<'PY'
import json
import os
from pathlib import Path

manifest = json.loads(Path(os.environ["RUNTIME_MANIFEST"]).read_text(encoding="utf-8"))
if manifest.get("os") != os.environ["TARGET_OS"] or manifest.get("arch") != os.environ["TARGET_ARCH"]:
    raise SystemExit(f"manifest targets {manifest.get('os')}/{manifest.get('arch')}, not {os.environ['TARGET_OS']}/{os.environ['TARGET_ARCH']}")
if not manifest.get("runtimeID"):
    raise SystemExit("manifest missing runtimeID")
PY

ASSET_DIR="${REPO_ROOT}/internal/embeddedpg/assets"
RUNTIME_DIR="${ASSET_DIR}/runtimes"
mkdir -p "$RUNTIME_DIR"
find "$RUNTIME_DIR" -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} +

archive_base="$(basename "$RUNTIME_ARCHIVE")"
manifest_base="$(basename "$RUNTIME_MANIFEST")"
cp "$RUNTIME_ARCHIVE" "${RUNTIME_DIR}/${archive_base}"
cp "$RUNTIME_MANIFEST" "${RUNTIME_DIR}/${manifest_base}"

cat > "${ASSET_DIR}/generated_runtime.go" <<EOF
package assets

var generatedRuntimeArchivePath = "runtimes/${archive_base}"
var generatedRuntimeManifestPath = "runtimes/${manifest_base}"
EOF

gofmt -w "${ASSET_DIR}/generated_runtime.go"
echo "Embedded runtime asset ${archive_base} for ${TARGET_OS}/${TARGET_ARCH}"
