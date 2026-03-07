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

# --- Optional pulse automation ---
# Enable recurring room-scoped automation tasks.
MATRIX_PULSE_ENABLED="false"

# How often manibot polls all configured room task lists.
MATRIX_PULSE_POLL_INTERVAL_SECONDS="300"

# Lease duration used to claim a room pulse run and prevent duplicate execution.
# Default behavior is MANIFOLD_REQUEST_TIMEOUT_SECONDS + 60 when unset.
MATRIX_PULSE_LEASE_SECONDS="240"

# Required when pulse is enabled unless DATABASE_URL / DB_URL / POSTGRES_DSN is already set.
PULSE_DATABASE_DSN="postgres://manifold:manifold@pg-manifold:5432/manifold?sslmode=disable"

# --- Optional Manifold endpoint settings ---
MANIFOLD_BASE_URL="http://localhost:32180"
MANIFOLD_PROMPT_PATH="/api/prompt"
MANIFOLD_PROJECT_ID=""
MANIFOLD_SYSTEM_PROMPT_FILE="./cmd/manibot/matrix-system-prompt.txt"
# Alternative for short prompts only:
# MANIFOLD_SYSTEM_PROMPT="You are Manifold in Matrix chat..."
MANIFOLD_SESSION_PREFIX="matrix"
MANIFOLD_REQUEST_TIMEOUT_SECONDS="180"
```

`manibot` sends a dedicated system prompt with each Matrix chat turn and pulse run.
If neither `MANIFOLD_SYSTEM_PROMPT_FILE` nor `MANIFOLD_SYSTEM_PROMPT` is set, it uses a built-in prompt tuned for plain-text Matrix conversations and scheduled pulse runs.

Use `MANIFOLD_SYSTEM_PROMPT_FILE` for real prompt customization. It is easier to maintain than a giant single-line env var.

## Pulse Mode

When `MATRIX_PULSE_ENABLED=true`, `manibot` starts a second loop alongside Matrix sync polling.

- The sync loop continues handling normal prefixed room messages.
- The pulse loop wakes up every `MATRIX_PULSE_POLL_INTERVAL_SECONDS` and checks the persisted room task lists.
- Tasks execute only when their own `interval_seconds` has elapsed, even if the bot polls more frequently.
- Pulse runs use a dedicated session per room, separate from manual chat history.
- Pulse runs do not post routine summaries back to the room.
- If a task explicitly needs to notify the room, the agent must use the `matrix_room_message` tool.
- The same Matrix-specific system prompt is used for both direct room chats and pulse runs, so the assistant voice stays consistent.

Pulse task state is stored in Postgres through `PULSE_DATABASE_DSN` or the normal shared database env vars. This is required for reliable leasing and multi-instance safety.

The agent manages tasks with the `pulse_tasks` tool. Supported actions are:

- `list`
- `configure_room`
- `upsert_task`
- `delete_task`
- `enable_task`
- `disable_task`
- `set_interval`

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
- Pulse continuity is also room-scoped, but uses a separate deterministic pulse session ID.
- `MANIFOLD_PROJECT_ID` is optional but useful when you want all room prompts to run against one project/workspace context.
- A room pulse can override the global project by storing a room-specific `project_id` through the `pulse_tasks` tool.
- If auth is enabled in `agentd`, configure either cookie or bearer env vars above.
