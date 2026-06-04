#!/usr/bin/env bash
set -euo pipefail

ARTIFACT=""
JSON_OUTPUT=""

usage() {
  echo "Usage: $0 --artifact <manifold-archive> [--json-output <path>]" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact) ARTIFACT="$2"; shift 2 ;;
    --json-output) JSON_OUTPUT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

[[ -f "$ARTIFACT" ]] || { usage; exit 2; }

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/manifold smoke.XXXXXX")"
HOME_DIR="${WORKDIR}/home"
APP_DIR="${WORKDIR}/app"
RUN_DIR="${WORKDIR}/run"
LOG_PATH="${WORKDIR}/manifold.log"
mkdir -p "$HOME_DIR" "$APP_DIR" "$RUN_DIR"

SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

case "$ARTIFACT" in
  *.zip) unzip -q "$ARTIFACT" -d "$APP_DIR" ;;
  *.tar.gz|*.tgz) tar -xzf "$ARTIFACT" -C "$APP_DIR" ;;
  *) echo "Unsupported artifact format: $ARTIFACT" >&2; exit 2 ;;
esac

MANIFOLD_BIN="$(find "$APP_DIR" -type f \( -name manifold -o -name manifold.exe \) | head -1)"
[[ -n "$MANIFOLD_BIN" ]] || { echo "manifold executable not found in artifact" >&2; exit 1; }
chmod +x "$MANIFOLD_BIN" || true

DOCTOR_JSON="${JSON_OUTPUT:-${WORKDIR}/doctor.json}"
HOME="$HOME_DIR" APPDATA="${HOME_DIR}/AppData/Roaming" "$MANIFOLD_BIN" embedded-postgres doctor --json > "$DOCTOR_JSON"
python3 - "$DOCTOR_JSON" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if not payload.get("ok"):
    raise SystemExit(payload.get("error", "doctor failed"))
PY

cat > "${RUN_DIR}/config.yaml" <<EOF
logPath: "${LOG_PATH}"
logLevel: info
llm_client:
  provider: openai
  openai:
    apiKey: dummy
    model: gpt-4o-mini
    api: completions
databases:
  embedded: true
  embeddedPort: 5433
  embeddedDataDir: ""
  defaultDSN: ""
  chat:
    backend: auto
    dsn: ""
  search:
    backend: auto
    dsn: ""
  vector:
    backend: auto
    dsn: ""
  graph:
    backend: auto
    dsn: ""
obs:
  otlp: ""
  local:
    enabled: true
  clickhouse:
    dsn: ""
EOF

(cd "$RUN_DIR" && HOME="$HOME_DIR" APPDATA="${HOME_DIR}/AppData/Roaming" "$MANIFOLD_BIN" >"${WORKDIR}/stdout.log" 2>"${WORKDIR}/stderr.log") &
SERVER_PID=$!

for _ in $(seq 1 90); do
  if curl -fsS http://127.0.0.1:32180/ >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    echo "manifold exited before HTTP smoke succeeded" >&2
    cat "${WORKDIR}/stderr.log" >&2 || true
    exit 1
  fi
  sleep 1
done

curl -fsS http://127.0.0.1:32180/ >/dev/null

if grep -Eiq 'extension not available|system discovery|apt|homebrew|fallback' "$LOG_PATH" "${WORKDIR}/stdout.log" "${WORKDIR}/stderr.log" 2>/dev/null; then
  echo "release smoke logs contain forbidden runtime fallback text" >&2
  exit 1
fi

echo "Manifold artifact smoke test passed for $ARTIFACT"
