# manifold

Manifold is an **experimental** platform for enabling long horizon workflow automation using teams of AI assistants. 

Manifold supports OpenAI models such as `gpt-5.2`, Google models such as `gemini-3-pro` and Anthropic models such as `claude-opus-4-5`. Manifold also supports OpenAI API compatible services for users that self host open weight models such as `gpt-oss-120b`, `devstral-2-123b` using [llama.cpp](https://github.com/ggml-org/llama.cpp) or [vllm](https://github.com/vllm-project/vllm).

Disclaimer: As an experimental frontier AI platform, we do not recommend Manifold be deployed in a production environment where stability is required until explicitly noted in this README. 

## Features

### **Agent Chat**
Use a traditional chat view to instruct specialists (agents) to work on objectives.

![chat](docs/img/chat.webp)

_Specialists can collaborate in multi-turn objectives. Manifold is designed to take advantage of the long horizon capabilities of frontier models and can work on complex objectives for hours._

### Image Generation

Manifold supports OpenAI and Google image generation models as well as local image generation using a custom ComfyUI MCP client.

![image generation](docs/img/imggen.webp)
_Example ComfyUI generated image with a custom workflow_

### **Specialist Registry**
Define and configure AI agents (specialists) and build your team of experts.

![specialists](docs/img/specialists.webp)

### **Projects**
Configure projects as agent workspaces.

![projects](docs/img/projects.webp)

### **Integrated tools and MCP Support**
Manifold implements internal tools for agent workflows as well as MCP support to extend the capabilities of your agents. Configure as many MCP servers as you wish. Enable tools individually to easily manage model context limits.

![specialists](docs/img/mcp.webp)

### **Workflow Editor**
Design agent workflows using a visual flow editor.

![specialists](docs/img/flow.webp)

### **Prompts, Datasets and Experiments Playground**
Create, iterate and version custom prompts that can be assigned to your agents. Configure datasets and run experiments to understand how prompts affect agent behaviors.

![specialists](docs/img/playground.webp)

## Quick Start

For step-by-step quick start instructions, see the repository Quick Start guide: [QUICKSTART.md](./QUICKSTART.md)

## API Docs

OpenAPI generation and API docs publishing workflow is documented in [docs/openapi.md](./docs/openapi.md).

## Matrix bot CLI (`manibot`)

Manifold includes a Matrix bot CLI that forwards room messages to the Manifold backend (`/api/prompt`), so responses can use your configured specialists, internal tools, MCP servers, and project skills.

Detailed setup, `.env` template, and a minimal Docker Compose service example are in [cmd/manibot/README.md](./cmd/manibot/README.md).

Run:

```bash
go run ./cmd/manibot
```

Required environment variables:

- `MATRIX_HOMESERVER_URL`
- `MATRIX_BOT_USER_ID`
- `MATRIX_ACCESS_TOKEN`

Optional variables:

- `BOT_PREFIX` (default: `!bot`)
- `MANIFOLD_BASE_URL` (default: `http://localhost:32180`)
- `MANIFOLD_PROMPT_PATH` (default: `/api/prompt`)
- `MANIFOLD_PROJECT_ID` (bind all prompts to one project/workspace)
- `MANIFOLD_SESSION_PREFIX` (default: `matrix`)
- `MANIFOLD_REQUEST_TIMEOUT_SECONDS` (default: `180`)
- `MATRIX_SYNC_TIMEOUT_SECONDS` (default: `30`)
- `MATRIX_SYNC_RETRY_DELAY_SECONDS` (default: `3`)
- `MATRIX_PULSE_ENABLED` (default: `false`; enables the background pulse loop)
- `MATRIX_PULSE_POLL_INTERVAL_SECONDS` (default: `300`; how often the bot checks all room task lists)
- `MATRIX_PULSE_LEASE_SECONDS` (default: `MANIFOLD_REQUEST_TIMEOUT_SECONDS + 60`; claim window to prevent duplicate pulse runs)
- `PULSE_DATABASE_DSN` (required when pulse is enabled unless one of `DATABASE_URL`, `DB_URL`, or `POSTGRES_DSN` is already set)
- `MANIFOLD_SESSION_COOKIE` and `MANIFOLD_SESSION_COOKIE_NAME` (for auth-enabled agentd cookie auth)
- `MANIFOLD_AUTH_BEARER_TOKEN` (optional bearer header passthrough)

### Matrix Pulse Automation

`manibot` can run a background pulse loop that checks room-scoped recurring tasks and submits due work to the Manifold orchestrator as an automated prompt.

Operator model:

- Pulse is room-scoped. Each Matrix room has its own task list.
- Pulse runs use a separate session from normal room chat, so automated context does not pollute human chat history.
- Due-task state is persisted in the shared database and claimed with a lease so multiple bot instances do not run the same room pulse simultaneously.
- The agent can manage tasks through the `pulse_tasks` tool during both manual chat runs and automated pulse runs.

Recommended setup:

- Enable pulse only when `manibot` can reach the same Postgres database used by Manifold persistence.
- Set `PULSE_DATABASE_DSN` explicitly in deployments where the bot process does not already inherit `DATABASE_URL`.
- Keep `MATRIX_PULSE_POLL_INTERVAL_SECONDS` shorter than the longest task cadence you care about; per-task intervals are enforced independently of the poll interval.

The `pulse_tasks` tool currently supports `list`, `configure_room`, `upsert_task`, `delete_task`, `enable_task`, `disable_task`, and `set_interval`.
