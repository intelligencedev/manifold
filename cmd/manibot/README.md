# manibot (Matrix bot for Manifold)

`manibot` listens in Matrix rooms and forwards prefixed messages to Manifold `/api/prompt`.
That means replies use your existing Manifold orchestrator setup (tools, MCP servers, specialists, and project skills).

## Run locally

```bash
go run ./cmd/manibot
```

`manibot` loads environment variables from `.env` automatically if present.

## `.env` template

```dotenv
# --- Required Matrix settings ---
MATRIX_HOMESERVER_URL="https://matrix.example.com"
MATRIX_BOT_USER_ID="@manibot:matrix.example.com"
MATRIX_ACCESS_TOKEN="paste_matrix_access_token_here"

# --- Optional bot behavior ---
BOT_PREFIX="!bot"
MATRIX_SYNC_TIMEOUT_SECONDS="30"
MATRIX_SYNC_RETRY_DELAY_SECONDS="3"

# --- Optional Manifold endpoint settings ---
MANIFOLD_BASE_URL="http://localhost:32180"
MANIFOLD_PROMPT_PATH="/api/prompt"
MANIFOLD_PROJECT_ID=""
MANIFOLD_SESSION_PREFIX="matrix"
MANIFOLD_REQUEST_TIMEOUT_SECONDS="180"

# --- Optional auth passthrough for auth-enabled agentd ---
# Cookie auth:
MANIFOLD_SESSION_COOKIE_NAME="sio_session"
MANIFOLD_SESSION_COOKIE=""

# Or bearer auth:
MANIFOLD_AUTH_BEARER_TOKEN=""
```

## Docker Compose snippet (minimal)

Add this service to your compose file (or a dedicated override file):

```yaml
services:
  manibot:
    image: golang:1.24
    container_name: manibot
    working_dir: /app
    command: ["go", "run", "./cmd/manibot"]
    env_file:
      - .env
    volumes:
      - ./:/app
    depends_on:
      - manifold
    restart: unless-stopped
```

Then run:

```bash
docker compose up -d manifold manibot
```

If your Matrix server is reachable only from host networking, set `MANIFOLD_BASE_URL` and Matrix URL values accordingly for container networking.

## Notes

- Session continuity is room-scoped: `manibot` maps each room to a deterministic Manifold `session_id`.
- `MANIFOLD_PROJECT_ID` is optional but useful when you want all room prompts to run against one project/workspace context.
- If auth is enabled in `agentd`, configure either cookie or bearer env vars above.
