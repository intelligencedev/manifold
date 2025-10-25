# Quick Start Deployment

NOTES: 
- Ensure docker is running.
- Ensure Node version 20 is installed and enabled. We recommend using Node Version Manager.
- Ensure pnpm is installed. Required for frontend build.
- OTEL Collector is not required despite the default config.

```
# To enable web search and fetch without SearXNG
# Ref: https://hub.docker.com/r/mcp/duckduckgo
$ docker pull mcp/duckduckgo

# Rename example env and config files
$ cp example.env .env && cp config.yaml.example config.yaml

# Configure real openai api key (OPENAI_API_KEY)
# Replace test123 with real key
$ sed -i '' 's/^OPENAI_API_KEY="[^"]*"/OPENAI_API_KEY="test123"/' .env

# Update submodules
$ git submodule update --init --recursive

# Install frontend dependencies
$ cd cd web/agentd-ui/
$ pnpm install
$ cd ../..

# Create log file
$ touch manifold.log

# Run minimal deployment (Takes a few minutes to build images)
$ docker compose up -d manifold pg-manifold
```

When containers are running, open browser and navigate to:

http://localhost:32180

Troubleshooting:

- Check `manifold.log`.
- Most common issue is database DSN. Ensure the configuration matches what docker compose configuration for `pg-manifold` service.