#!/usr/bin/env bash
set -euo pipefail

RUNTIME_ARCHIVE=""
RUNTIME_MANIFEST=""
JSON_OUTPUT=""

usage() {
  echo "Usage: $0 --runtime-archive <path> --runtime-manifest <path> [--json-output <path>]" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --runtime-archive) RUNTIME_ARCHIVE="$2"; shift 2 ;;
    --runtime-manifest) RUNTIME_MANIFEST="$2"; shift 2 ;;
    --json-output) JSON_OUTPUT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

[[ -n "$RUNTIME_ARCHIVE" ]] || { usage; exit 2; }
[[ -n "$RUNTIME_MANIFEST" ]] || { usage; exit 2; }

WORKDIR="$(mktemp -d)"
RUNTIME_DIR="${WORKDIR}/runtime"
DATA_DIR="${WORKDIR}/data"
SOCKET_DIR="${WORKDIR}/socket"
LOG_PATH="${WORKDIR}/postgres.log"
mkdir -p "$RUNTIME_DIR" "$DATA_DIR" "$SOCKET_DIR"

PG_CTL_PID_STARTED=0
PG_CTL=""
cleanup() {
  if [[ "$PG_CTL_PID_STARTED" == "1" && -n "$PG_CTL" && -x "$PG_CTL" ]]; then
    "$PG_CTL" -D "$DATA_DIR" stop -m fast >/dev/null 2>&1 || true
  fi
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

write_json() {
  local ok="$1" error="$2"
  [[ -n "$JSON_OUTPUT" ]] || return 0
  export RUNTIME_MANIFEST ok error LOG_PATH
  mkdir -p "$(dirname "$JSON_OUTPUT")"
  python3 - <<'PY' > "$JSON_OUTPUT"
import json
import os
from pathlib import Path

manifest = json.loads(Path(os.environ["RUNTIME_MANIFEST"]).read_text(encoding="utf-8"))
payload = {
    "ok": os.environ["ok"] == "true",
    "runtimeID": manifest.get("runtimeID", ""),
    "os": manifest.get("os", ""),
    "arch": manifest.get("arch", ""),
    "postgres": manifest.get("postgres", {}),
    "extensions": manifest.get("extensions", {}),
    "logPath": os.environ["LOG_PATH"],
}
if os.environ["error"]:
    payload["error"] = os.environ["error"]
print(json.dumps(payload, indent=2, sort_keys=True))
PY
}

fail() {
  write_json false "$1"
  echo "$1" >&2
  exit 1
}

case "$RUNTIME_ARCHIVE" in
  *.zip) unzip -q "$RUNTIME_ARCHIVE" -d "$RUNTIME_DIR" ;;
  *.tar.zst) command -v zstd >/dev/null 2>&1 || fail "zstd is required"; zstd -dc "$RUNTIME_ARCHIVE" | tar -xf - -C "$RUNTIME_DIR" ;;
  *.tar.gz|*.tgz) tar -xzf "$RUNTIME_ARCHIVE" -C "$RUNTIME_DIR" ;;
  *) fail "unsupported runtime archive format: $RUNTIME_ARCHIVE" ;;
esac

if [[ -x "${RUNTIME_DIR}/bin/pg_ctl.exe" ]]; then
  PG_CTL="${RUNTIME_DIR}/bin/pg_ctl.exe"
  INITDB="${RUNTIME_DIR}/bin/initdb.exe"
  PSQL="${RUNTIME_DIR}/bin/psql.exe"
else
  PG_CTL="${RUNTIME_DIR}/bin/pg_ctl"
  INITDB="${RUNTIME_DIR}/bin/initdb"
  PSQL="${RUNTIME_DIR}/bin/psql"
fi

[[ -x "$PG_CTL" ]] || fail "pg_ctl not executable in runtime"
[[ -x "$INITDB" ]] || fail "initdb not executable in runtime"
[[ -x "$PSQL" ]] || fail "psql not executable in runtime"

PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"

export PATH="${RUNTIME_DIR}/bin:${PATH}"
export LD_LIBRARY_PATH="${RUNTIME_DIR}/lib:${RUNTIME_DIR}/lib/postgresql:${LD_LIBRARY_PATH:-}"
export DYLD_LIBRARY_PATH="${RUNTIME_DIR}/lib:${RUNTIME_DIR}/lib/postgresql:${DYLD_LIBRARY_PATH:-}"

"$INITDB" -D "$DATA_DIR" -U manifold -A trust --no-locale >/dev/null || fail "initdb failed"
"$PG_CTL" -D "$DATA_DIR" -l "$LOG_PATH" -o "-h 127.0.0.1 -p ${PORT} -k ${SOCKET_DIR}" start >/dev/null || fail "postgres start failed"
PG_CTL_PID_STARTED=1

export PGHOST=127.0.0.1
export PGPORT="$PORT"
export PGUSER=manifold
export PGDATABASE=postgres

"$PSQL" -v ON_ERROR_STOP=1 -c "CREATE EXTENSION IF NOT EXISTS vector;" >/dev/null || fail "CREATE EXTENSION vector failed"
"$PSQL" -v ON_ERROR_STOP=1 -c "CREATE EXTENSION IF NOT EXISTS postgis;" >/dev/null || fail "CREATE EXTENSION postgis failed"
"$PSQL" -v ON_ERROR_STOP=1 -c "CREATE EXTENSION IF NOT EXISTS pgrouting;" >/dev/null || fail "CREATE EXTENSION pgrouting failed"
"$PSQL" -v ON_ERROR_STOP=1 -At -c "SELECT postgis_full_version();" >/dev/null || fail "postgis_full_version probe failed"
"$PSQL" -v ON_ERROR_STOP=1 -At -c "SELECT pgr_version();" >/dev/null || fail "pgr_version probe failed"

"$PG_CTL" -D "$DATA_DIR" stop -m fast >/dev/null || fail "postgres stop failed"
PG_CTL_PID_STARTED=0
write_json true ""
echo "Runtime smoke test passed for $RUNTIME_ARCHIVE"
