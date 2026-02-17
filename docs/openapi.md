# OpenAPI and API Docs

This project now includes:

- A generated OpenAPI spec (`docs/openapi/openapi.json`)
- A runtime Swagger UI page in `agentd`
- A static Swagger UI page (`docs/openapi/index.html`) for publishing

## Runtime API Docs (Local Testing)

Start `agentd`, then open:

- `http://localhost:32180/api-docs`
- `http://localhost:32180/openapi.json`

`/api-docs` uses Swagger UI with "Try it out", so users can execute requests directly against the running server.

## Generate OpenAPI Spec

Use the Make target:

```bash
make openapi
```

This runs:

```bash
go run ./cmd/openapi -out docs/openapi/openapi.json -server http://localhost:32180
```

You can override values manually:

```bash
go run ./cmd/openapi \
  -out docs/openapi/openapi.json \
  -server https://api.example.com \
  -auth \
  -auth-cookie manifold_session
```

## Publish Static API Page

The static page is `docs/openapi/index.html`. It loads `./openapi.json`.

Typical flow:

1. Run `make openapi`
2. Commit `docs/openapi/openapi.json` (and any docs updates)
3. Publish `docs/openapi/` via your static hosting (for example GitHub Pages)

The page supports overrides via query parameters:

- `?spec=<url>`: alternate OpenAPI spec URL
- `?server=<url>`: force target API server in Swagger UI (useful when testing prod/staging)

Example:

```text
https://<your-docs-host>/openapi/index.html?server=https://api.example.com
```

## Keeping Spec Current

When API routes change:

1. Update `internal/apidocs/spec.go` route metadata
2. Regenerate with `make openapi`
3. Commit the generated spec and related docs changes
