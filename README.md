<p align="center">
<!-- TODO: replace logo asset file when new branding asset is available -->
<img src="assets/singularityio-logo.svg" alt="manifold logo (update asset)" width="200" />
</p>

# manifold

Manifold is a platform for creating and managing AI assistants.

## Features

- **Two Binaries**: `agent` (CLI) and `agentd` (HTTP server with web UI)
- **Safe Execution**: Tool calling with secure executor (no shell) in locked WORKDIR
- **Built-in Tools**: run_cli, web_search, web_fetch, write_file, llm_transform
- **Database Integration**: PostgreSQL with PGVector, PostGIS, and PGRouting
- **MCP Support**: Model Context Protocol client for external tool providers
- **Observability**: Structured logging, OpenTelemetry traces and metrics
- **Auth/RBAC**: OIDC authentication and RBAC support

## Requirements

- Go 1.21+ (recommended: Go 1.24+)
- OpenAI API key or compatible local or public endpoint
- PostgreSQL with required extensions. We recommend deploying a Postgres instance using the Dockerfile at `deploy/docker/postgres.Dockerfile`.
- Google Chrome (or a Chromium-compatible browser) installed — required for the web tools to work.

## Quick Start

For step-by-step quick start instructions, see the repository Quick Start guide: [QUICKSTART.md](./QUICKSTART.md)

## Documentation

- [Configuration](docs/configuration.md) - Environment variables and YAML configuration
- [Running](docs/running.md) - Detailed usage instructions for agent and agentd
- [Tools](docs/tools.md) - Built-in and custom tools
- [Database](docs/database.md) - Database setup and configuration
- [MCP Client](docs/mcp.md) - Model Context Protocol integration
- [Specialists & Routing](docs/specialists.md) - Route requests to specialized endpoints
- [Security](docs/security.md) - Security features and best practices
- [Observability](docs/observability.md) - Logging, tracing, and monitoring
- [Development](docs/development.md) - Development setup and contributing
- [Authentication](docs/auth.md) - OIDC and session management
