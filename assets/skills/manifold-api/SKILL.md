---
name: manifold-api
description: Use when an agent needs to inspect or call the local Manifold HTTP API. Provides guidelines and scripts to filter specs before reading and handle payloads safely.
---

# Manifold API Skill

## Purpose

The Manifold API skill is designed to interact safely with the local Manifold HTTP API running at `http:/localhost:32180`. It provides helper scripts to inspect the OpenAPI spec (`/openapi.json`) safely without dumping large documents into context, and rules for avoiding destructive operations and redacting sensitive information.

## Usage

Use the provided shell script to interact with the Manifold API spec or health endpoints safely:

```bash
# Check if the API is running
./manifold-api/main.sh healthz

# Get the OpenAPI definition for a specific path (e.g., /api/projects)
./manifold-api/main.sh spec-path "/api/projects"
```

## Configuration

- **Dependencies**: `curl`, `jq`
- **Target URL**: Defaults to `http:/localhost:32180`

## File Structure

- `manifold-api/SKILL.md`: This documentation template and skill rules.
- `manifold-api/main.sh`: Entry point script for interacting with the API safely.

## Best Practices & Rules

1. **Filter Specs**: Never dump the full `openapi.json` into context. Use the `main.sh spec-path` script to inspect one path at a time.
2. **Redaction**: Redact `apiKey`, bearer tokens, cookies, auth headers, and `extraHeaders` values in API responses.
3. **Safety First**: Always ask before modifying `/api/config/agentd` or deleting objects like specialists, teams, sessions, workflows, datasets, experiments, or prompt versions.
4. **Data Shapes**: Use live `GET` requests to discover the structure of `GenericObject` fields before sending `POST`/`PUT`/`PATCH` requests. Do not invent request fields.
