# agentd Frontend

The agentd dashboard is a Vue 3 single-page application located in `web/agentd-ui`. It ships alongside the Go backend by compiling the app with Vite and embedding the resulting static assets into the `agentd` binary.

## Prerequisites

- Node.js 20+
- [pnpm](https://pnpm.io/) 9+

Install dependencies:

```bash
cd web/agentd-ui
pnpm install
```

## Development Workflow

- Run `make dev-frontend` (or `pnpm dev`) for hot-reload development on port `5173`.
- Start `agentd` with `FRONTEND_DEV_PROXY=http://localhost:5173` to proxy all UI traffic directly to the dev server while keeping API routes served by Go.
- Shared mock data lives under `web/agentd-ui/src/mocks` for Storybook-style demo states.

## Building

Run `make frontend` to compile the UI. The target copies the contents of `web/agentd-ui/dist` into `internal/webui/dist` where they are embedded by the Go compiler. Follow with `make build-agentd-whisper` (or `make build`) to produce the binary. You can build manually with:

```bash
cd web/agentd-ui
pnpm build
```

The output is emitted under `web/agentd-ui/dist` and embedded via `//go:embed` at build time.

## Testing & Quality

- Unit tests: `pnpm test:unit` (Vitest)
- Linting & formatting:
  - `pnpm lint`
  - `pnpm format`
- End-to-end smoke tests: `pnpm test:e2e` (Playwright) – requires `pnpm preview`

## Troubleshooting

- If `agentd` logs `frontend registration failed`, ensure `internal/webui/dist/index.html` exists. Run `make frontend` to rebuild assets.
- To use on-disk assets without embedding, compile with `-tags dev` and set `AGENTD_WEB_DIST=/absolute/path/to/web/agentd-ui/dist`.
- Clear cached assets by removing `web/agentd-ui/dist` and `internal/webui/dist` before a clean rebuild.

## Chat Sessions & Authentication

The chat UI now loads conversations and message history from the backend APIs
(`/api/chat/sessions` and `/api/chat/sessions/{id}/messages`) instead of relying
on `localStorage`. When Auth is enabled the browser must send the session cookie
(`withCredentials` is already configured in `apiClient`). Users without the
`admin` role only see their own conversations; admins receive the full list.
If the backend responds with `401/403`, the sidebar surfaces the access message
and no local fallback data is used.
