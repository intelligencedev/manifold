# Running manifold

manifold provides two main binaries:
- `agent`: CLI application for command-line usage
- `agentd`: HTTP server with embedded Vue.js web UI

## CLI (agent)

Basic usage:

```bash
go run ./cmd/agent -q "List files and print README.md if present"
```

### Options

- `-q, --query`: Query string to execute
- `-w, --workdir`: Working directory (overrides WORKDIR env var)
- `-v, --verbose`: Enable verbose output
- `--config`: Path to config file (default: config.yaml)

## HTTP server / Web UI (agentd)

Start the web server:

```bash
# Optional: WEB UI settings
FRONTEND_DEV_PROXY=http://localhost:5173  # if running dev server
AGENTD_HTTP_HOST=0.0.0.0                  # default localhost
AGENTD_HTTP_PORT=32180                    # default 32180

# run directly (reads .env)
go run ./cmd/agentd

# Or set environment inline
AGENTD_HTTP_HOST=0.0.0.0 go run ./cmd/agentd
```

### Mock Development Mode

For frontend development without requiring a real LLM:

```bash
MOCK_MODE=true go run ./cmd/agentd
```

- `agentd` will return HTTP 500 if the configured LLM provider returns an error (for example, an unsupported parameter). For local UI testing, use the mock dev mode above or ensure your OpenAI-compatible endpoint supports all parameters being sent.

### Web UI Access

Once running, access the web interface at:
- Local: http://localhost:32180
- Custom host/port: http://AGENTD_HTTP_HOST:AGENTD_HTTP_PORT

## WARPP mode

WARPP is a workflow executor pattern: Intent detection → personalization → fulfillment with tool allow-listing.

Start WARPP mode:

```bash
go run ./cmd/agentd -warpp
```

## embedctl

Standalone embedding utility for generating text embeddings:

```bash
go run ./cmd/embedctl "text to embed"
```

## Docker

### Build and run manually

```bash
# Build the server image
docker build -t agentd .

# Run
docker run -it --env-file .env -p 32180:32180 agentd
```

### Docker Compose

```bash
cd deploy/docker
docker compose up -d pg-manifold manifold
```

This starts:
- `pg-manifold`: PostgreSQL database with PGVector, PostGIS, and PGRouting extensions
- `manifold`: The agent runtime HTTP server with web UI