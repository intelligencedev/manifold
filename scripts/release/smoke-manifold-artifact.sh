#!/usr/bin/env bash
set -euo pipefail

ARTIFACT=""
JSON_OUTPUT=""
TARGET_OS=""
TARGET_ARCH=""

usage() {
  echo "Usage: $0 --artifact <manifold-archive> [--target-os <os>] [--target-arch <arch>] [--json-output <path>]" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --artifact) ARTIFACT="$2"; shift 2 ;;
    --target-os) TARGET_OS="$2"; shift 2 ;;
    --target-arch) TARGET_ARCH="$2"; shift 2 ;;
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
mkdir -p "$HOME_DIR" "$APP_DIR" "$RUN_DIR" "${RUN_DIR}/workdir"

SERVER_PID=""
EMBED_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$EMBED_PID" ]]; then
    kill "$EMBED_PID" >/dev/null 2>&1 || true
    wait "$EMBED_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$WORKDIR"
}
trap 'status=$?; cleanup; exit "$status"' EXIT

case "$ARTIFACT" in
  *.zip) unzip -q "$ARTIFACT" -d "$APP_DIR" ;;
  *.tar.gz|*.tgz) tar -xzf "$ARTIFACT" -C "$APP_DIR" ;;
  *) echo "Unsupported artifact format: $ARTIFACT" >&2; exit 2 ;;
esac

MANIFOLD_BIN="$(find "$APP_DIR" -type f \( -name manifold -o -name manifold.exe \) | head -1)"
[[ -n "$MANIFOLD_BIN" ]] || { echo "manifold executable not found in artifact" >&2; exit 1; }
chmod +x "$MANIFOLD_BIN" || true

for required in config.yaml.example specialists.yaml.example mcp.yaml.example example.env THIRD_PARTY_NOTICES.txt; do
  [[ -f "$APP_DIR/$required" ]] || { echo "required release file not found in artifact: $required" >&2; exit 1; }
done

DOCTOR_JSON="${JSON_OUTPUT:-${WORKDIR}/doctor.json}"

host_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) uname -s | tr '[:upper:]' '[:lower:]' ;;
  esac
}

host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) uname -m ;;
  esac
}

HOST_OS="$(host_os)"
HOST_ARCH="$(host_arch)"

if [[ -n "$TARGET_OS" && -n "$TARGET_ARCH" && ( "$TARGET_OS" != "$HOST_OS" || "$TARGET_ARCH" != "$HOST_ARCH" ) ]]; then
  python3 - "$DOCTOR_JSON" "$TARGET_OS" "$TARGET_ARCH" "$HOST_OS" "$HOST_ARCH" <<'PY'
import json
import sys

output, target_os, target_arch, host_os, host_arch = sys.argv[1:]
payload = {
    "ok": True,
    "executionSkipped": True,
    "reason": "artifact target is not native to the self-hosted runner",
    "targetOS": target_os,
    "targetArch": target_arch,
    "hostOS": host_os,
    "hostArch": host_arch,
}
with open(output, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, indent=2)
    handle.write("\n")
PY
  echo "Validated package contents for ${TARGET_OS}/${TARGET_ARCH}; skipped executable smoke on ${HOST_OS}/${HOST_ARCH}"
  exit 0
fi

HOME="$HOME_DIR" APPDATA="${HOME_DIR}/AppData/Roaming" \
  "$MANIFOLD_BIN" storage doctor --json --path "${HOME_DIR}/.manifold/manifold.db" > "$DOCTOR_JSON"
python3 - "$DOCTOR_JSON" <<'PY'
import json
import sys
payload = json.load(open(sys.argv[1], encoding="utf-8"))
if not payload.get("ok"):
    raise SystemExit(payload.get("error", "doctor failed"))
PY

cat > "${WORKDIR}/embedding_stub.py" <<'PY'
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        inputs = payload.get("input", [])
        if isinstance(inputs, str):
            inputs = [inputs]
        body = json.dumps({
            "object": "list",
            "model": payload.get("model", "stub"),
            "data": [
                {"object": "embedding", "index": i, "embedding": [0.0] * 1536}
                for i, _ in enumerate(inputs)
            ],
        }).encode("utf-8")
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        return


server = HTTPServer(("127.0.0.1", 0), Handler)
with open(sys.argv[1], "w", encoding="utf-8") as handle:
    handle.write(str(server.server_port))
server.serve_forever()
PY

EMBED_PORT_FILE="${WORKDIR}/embedding_stub.port"
python3 "${WORKDIR}/embedding_stub.py" "$EMBED_PORT_FILE" &
EMBED_PID=$!
for _ in $(seq 1 50); do
  [[ -s "$EMBED_PORT_FILE" ]] && break
  sleep 0.1
done
[[ -s "$EMBED_PORT_FILE" ]] || { echo "embedding stub did not start" >&2; exit 1; }
EMBED_BASE_URL="http://127.0.0.1:$(cat "$EMBED_PORT_FILE")"

cat > "${RUN_DIR}/config.yaml" <<EOF
workdir: "${RUN_DIR}/workdir"
logPath: "${LOG_PATH}"
logLevel: info
llm_client:
  provider: openai
  openai:
    apiKey: dummy
    model: gpt-4o-mini
    api: completions
embedding:
  baseURL: "${EMBED_BASE_URL}"
  model: text-embedding-3-small
  apiKey: dummy
  apiHeader: Authorization
  path: /v1/embeddings
  timeoutSeconds: 5
databases:
  backend: sqlite
  sqlite:
    path: "${HOME_DIR}/.manifold/manifold.db"
  chat:
    backend: sqlite
  search:
    backend: sqlite
  vector:
    backend: sqlite
  graph:
    backend: sqlite
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
    cat "${WORKDIR}/stdout.log" >&2 || true
    cat "${LOG_PATH}" >&2 || true
    exit 1
  fi
  sleep 1
done

curl -fsS http://127.0.0.1:32180/ >/dev/null

echo "Manifold artifact smoke test passed for $ARTIFACT"
