#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PG_VERSION=""
PG_MAJOR=""
PGVECTOR_VERSION=""
POSTGIS_VERSION=""
PGROUTING_VERSION=""
TARGET_OS=""
TARGET_ARCH=""
OUTPUT_DIR="${REPO_ROOT}/dist/postgres-runtime"
RUNTIME_ROOT="${POSTGRES_RUNTIME_ROOT:-}"

usage() {
  cat <<'USAGE'
Usage: build-postgres-runtime.sh --pg-version <17.x> --pg-major 17 \
  --pgvector-version 0.8.2 --postgis-version 3.6.2 --pgrouting-version 4.0.1 \
  --target-os linux|darwin|windows --target-arch amd64|arm64 \
  [--output-dir dist/postgres-runtime] [--runtime-root <path>]

The script assembles a release runtime from --runtime-root, or from the local
pg_config installation when the host platform matches the target platform.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pg-version) PG_VERSION="$2"; shift 2 ;;
    --pg-major) PG_MAJOR="$2"; shift 2 ;;
    --pgvector-version) PGVECTOR_VERSION="$2"; shift 2 ;;
    --postgis-version) POSTGIS_VERSION="$2"; shift 2 ;;
    --pgrouting-version) PGROUTING_VERSION="$2"; shift 2 ;;
    --target-os) TARGET_OS="$2"; shift 2 ;;
    --target-arch) TARGET_ARCH="$2"; shift 2 ;;
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    --runtime-root) RUNTIME_ROOT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

required() {
  local name="$1" value="$2"
  if [[ -z "$value" ]]; then
    echo "Missing required option: $name" >&2
    usage
    exit 2
  fi
}

required "--pg-version" "$PG_VERSION"
required "--pg-major" "$PG_MAJOR"
required "--pgvector-version" "$PGVECTOR_VERSION"
required "--postgis-version" "$POSTGIS_VERSION"
required "--pgrouting-version" "$PGROUTING_VERSION"
required "--target-os" "$TARGET_OS"
required "--target-arch" "$TARGET_ARCH"

case "$TARGET_OS" in linux|darwin|windows) ;; *) echo "Unsupported target OS: $TARGET_OS" >&2; exit 2 ;; esac
case "$TARGET_ARCH" in amd64|arm64) ;; *) echo "Unsupported target arch: $TARGET_ARCH" >&2; exit 2 ;; esac

host_os() {
  case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
    linux*) echo linux ;;
    darwin*) echo darwin ;;
    mingw*|msys*|cygwin*) echo windows ;;
    *) uname -s | tr '[:upper:]' '[:lower:]' ;;
  esac
}

host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    arm64|aarch64) echo arm64 ;;
    *) uname -m ;;
  esac
}

RUNTIME_ID="postgres-${PG_VERSION}-pgvector-${PGVECTOR_VERSION}-postgis-${POSTGIS_VERSION}-pgrouting-${PGROUTING_VERSION}-${TARGET_OS}-${TARGET_ARCH}"
STAGING="$(mktemp -d)"
trap 'rm -rf "$STAGING"' EXIT

copy_dir_if_exists() {
  local src="$1" dst="$2"
  if [[ -d "$src" ]]; then
    mkdir -p "$dst"
    cp -R -L "$src"/. "$dst"/
  fi
}

copy_file_if_exists() {
  local src="$1" dst="$2"
  if [[ -f "$src" ]]; then
    mkdir -p "$(dirname "$dst")"
    cp -L "$src" "$dst"
  fi
}

assemble_from_runtime_root() {
  local root="$1"
  [[ -d "$root" ]] || { echo "Runtime root does not exist: $root" >&2; exit 1; }
  copy_dir_if_exists "$root/bin" "${STAGING}/bin"
  copy_dir_if_exists "$root/lib" "${STAGING}/lib"
  copy_dir_if_exists "$root/share" "${STAGING}/share"
  copy_dir_if_exists "$root/licenses" "${STAGING}/licenses"
}

assemble_from_pg_config() {
  command -v pg_config >/dev/null 2>&1 || {
    echo "pg_config not found. Install PostgreSQL ${PG_MAJOR} with pgvector/PostGIS/pgRouting, or pass --runtime-root." >&2
    exit 1
  }
  local bindir libdir pkglibdir sharedir
  bindir="$(pg_config --bindir)"
  libdir="$(pg_config --libdir)"
  pkglibdir="$(pg_config --pkglibdir)"
  sharedir="$(pg_config --sharedir)"

  mkdir -p "${STAGING}/bin" "${STAGING}/lib/postgresql" "${STAGING}/share/postgresql" "${STAGING}/licenses"
  for bin in postgres initdb pg_ctl psql createdb; do
    copy_file_if_exists "${bindir}/${bin}" "${STAGING}/bin/${bin}"
    copy_file_if_exists "${bindir}/${bin}.exe" "${STAGING}/bin/${bin}.exe"
  done
  copy_dir_if_exists "$libdir" "${STAGING}/lib"
  copy_dir_if_exists "$pkglibdir" "${STAGING}/lib"
  copy_dir_if_exists "$pkglibdir" "${STAGING}/lib/postgresql"
  copy_dir_if_exists "$sharedir" "${STAGING}/share/postgresql"

  for doc in "/usr/share/doc/postgresql"* "/usr/local/share/doc/postgresql"*; do
    if [[ -e "$doc" ]]; then
      copy_dir_if_exists "$doc" "${STAGING}/licenses/$(basename "$doc")"
    fi
  done
}

if [[ -n "$RUNTIME_ROOT" ]]; then
  assemble_from_runtime_root "$RUNTIME_ROOT"
else
  if [[ "$(host_os)" != "$TARGET_OS" || "$(host_arch)" != "$TARGET_ARCH" ]]; then
    echo "Cross-target assembly requires --runtime-root or POSTGRES_RUNTIME_ROOT." >&2
    exit 1
  fi
  assemble_from_pg_config
fi

PG_CTL="pg_ctl"
POSTGRES_BIN="postgres"
if [[ "$TARGET_OS" == "windows" ]]; then
  PG_CTL="pg_ctl.exe"
  POSTGRES_BIN="postgres.exe"
fi

for required_file in "bin/${POSTGRES_BIN}" "bin/initdb${POSTGRES_BIN#postgres}" "bin/${PG_CTL}" "share/postgresql/extension/vector.control" "share/postgresql/extension/postgis.control" "share/postgresql/extension/pgrouting.control"; do
  if [[ ! -f "${STAGING}/${required_file}" ]]; then
    echo "Runtime is missing required file: ${required_file}" >&2
    exit 1
  fi
done

mkdir -p "$OUTPUT_DIR"
MANIFEST_PATH="${OUTPUT_DIR}/postgres-runtime-${RUNTIME_ID}.manifest.json"
export STAGING RUNTIME_ID TARGET_OS TARGET_ARCH PG_MAJOR PG_VERSION PGVECTOR_VERSION POSTGIS_VERSION PGROUTING_VERSION MANIFEST_PATH
python3 - <<'PY'
import hashlib
import json
import os
from pathlib import Path

staging = Path(os.environ["STAGING"])
files = []
for path in sorted(p for p in staging.rglob("*") if p.is_file()):
    rel = path.relative_to(staging).as_posix()
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    mode = f"{path.stat().st_mode & 0o777:04o}"
    files.append({"path": rel, "mode": mode, "sha256": digest})

manifest = {
    "schemaVersion": 1,
    "runtimeID": os.environ["RUNTIME_ID"],
    "os": os.environ["TARGET_OS"],
    "arch": os.environ["TARGET_ARCH"],
    "postgres": {"major": int(os.environ["PG_MAJOR"]), "version": os.environ["PG_VERSION"]},
    "extensions": {
        "vector": os.environ["PGVECTOR_VERSION"],
        "postgis": os.environ["POSTGIS_VERSION"],
        "pgrouting": os.environ["PGROUTING_VERSION"],
    },
    "dependencies": [],
    "files": files,
}
manifest_path = Path(os.environ["MANIFEST_PATH"])
manifest_path.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
(staging / "manifest.json").write_text(manifest_path.read_text(encoding="utf-8"), encoding="utf-8")
PY

if [[ "$TARGET_OS" == "windows" ]]; then
  ARCHIVE_PATH="${OUTPUT_DIR}/postgres-runtime-${RUNTIME_ID}.zip"
  (cd "$STAGING" && zip -qr "$ARCHIVE_PATH" .)
else
  command -v zstd >/dev/null 2>&1 || { echo "zstd is required to create tar.zst runtime archives" >&2; exit 1; }
  ARCHIVE_PATH="${OUTPUT_DIR}/postgres-runtime-${RUNTIME_ID}.tar.zst"
  tar -cf - -C "$STAGING" . | zstd -T0 -19 -f -o "$ARCHIVE_PATH" >/dev/null
fi

SHA_PATH="${ARCHIVE_PATH}.sha256"
shasum -a 256 "$ARCHIVE_PATH" > "$SHA_PATH"

if [[ "${SKIP_RUNTIME_SMOKE:-0}" != "1" ]]; then
  "${SCRIPT_DIR}/test-postgres-runtime.sh" \
    --runtime-archive "$ARCHIVE_PATH" \
    --runtime-manifest "$MANIFEST_PATH" \
    --json-output "${OUTPUT_DIR}/runtime-smoke-${RUNTIME_ID}.json"
fi

echo "Created runtime archive: $ARCHIVE_PATH"
echo "Created runtime manifest: $MANIFEST_PATH"
echo "Created checksum: $SHA_PATH"
