# Component: Frontend (`web/agentd-ui`)

The frontend is a Vue 3 app that can run separately during development or be embedded into `agentd` for production-style runs.

## Stack

- Vue 3
- Vite 6
- TypeScript
- Pinia
- Vue Router
- TanStack Vue Query
- Vue Flow for workflow editing
- Tailwind CSS
- Vitest and Playwright

## Main Files

| File/path | Purpose |
| --- | --- |
| `web/agentd-ui/package.json` | Scripts, dependencies, Node/pnpm constraints. |
| `web/agentd-ui/src/main.ts` | App bootstrap. |
| `web/agentd-ui/src/App.vue` | Top-level shell, nav, account button, feature-gated nav. |
| `web/agentd-ui/src/router/index.ts` | UI route table. |
| `web/agentd-ui/src/api/*` | Backend API clients. |
| `web/agentd-ui/src/stores/*` | Pinia state stores. |
| `web/agentd-ui/src/views/*` | Page-level route views. |
| `web/agentd-ui/src/components/*` | Shared and feature components. |
| `web/agentd-ui/tests`, `e2e` | Unit and E2E tests. |

## Route Surface

```mermaid
flowchart LR
    App[App.vue] --> Overview[/]
    App --> Projects[/projects]
    App --> Specialists[/specialists]
    App --> Chat[/chat]
    App --> Pulse[/pulse]
    App --> Flow[/flow]
    App --> Playground[/playground]
    App --> Settings[/settings]
    App --> Beta[beta feature gate]
    Beta --> CodeQA[/codeqa]
    Beta --> Beliefs[/beliefs]
```

## API Client Pattern

Frontend API modules generally use a shared `apiClient` from `src/api/client.ts` and map backend routes to typed functions. Chat streaming uses `fetch` and parses server-sent events rather than ordinary Axios JSON calls.

## Embedded Build

`make frontend` builds the Vue app and copies assets for Go embedding. `make build-manifold` builds `agentd` with embedded frontend assets. The Dockerfile performs the same multi-stage UI build before compiling the backend binary.

## Contributor Guidance

- If changing backend route paths or payloads, update `src/api/*` and affected stores/views.
- If adding a new view, update `src/router/index.ts`, `App.vue` nav, and tests.
- Use the feature gate pattern for experimental nav items.
- For Flow editor changes, keep backend `internal/flow/types.go` and frontend `src/types/flowV2.ts` aligned.
- Keep stream event names in sync with `src/api/chat.ts`.

## Evidence

- `web/agentd-ui/package.json` defines dependencies and scripts.
- `web/agentd-ui/src/router/index.ts` defines route table.
- `web/agentd-ui/src/App.vue` defines top navigation and beta feature gate.
- `web/agentd-ui/README.md` documents local frontend development and embedding.
- `deploy/docker/cpu.Dockerfile` builds frontend assets before backend.
