# Interface: HTTP API

`agentd` exposes HTTP routes through `internal/agentd/router.go`. Generated OpenAPI metadata lives in `internal/apidocs/spec.go` and can be served at runtime.

## Route Groups

| Group | Routes | Backend files |
| --- | --- | --- |
| Health/docs | `/healthz`, `/readyz`, `/openapi.json`, `/api/openapi.json`, `/api-docs`, `/api/docs` | `router.go`, `handlers_openapi.go`, `internal/apidocs/spec.go` |
| Auth/users | `/auth/login`, `/auth/callback`, `/auth/logout`, `/api/me`, `/api/users`, `/api/users/{id}` | `handlers_auth.go`, `internal/auth/*` |
| Preferences | `/api/me/preferences`, `/api/me/preferences/project` | `handlers_preferences.go` |
| Projects | `/api/projects`, `/api/projects/{project_id}`, tree/files/dirs/move/archive subroutes | `handlers_projects.go`, `internal/projects/service.go` |
| Chat/runs | `/api/runs`, `/api/chat/sessions`, session messages/activities/title, `/api/prompt`, `/agent/run`, `/agent/vision` | `handlers_chat.go`, `chat_*.go` |
| Specialists/teams | `/api/specialists/defaults`, `/api/specialists`, `/api/specialists/{name}`, `/api/teams`, team member routes | `handlers_specialists.go`, `handlers_teams.go` |
| Flow v2 | `/api/flows/v2/tools`, workflows, validate, run, run events | `handlers_flow_v2.go`, `flow_v2_runtime.go` |
| MCP | `/api/mcp/servers`, `/api/mcp/servers/{name}`, OAuth start/bootstrap/callback | `handlers_mcp.go`, `internal/mcpclient/*` |
| Transit | `/api/transit/memories`, memory detail, keys, recent, search, discover | `handlers_transit.go`, `internal/transit/*` |
| Debug memory/beliefs | `/debug/memory`, `/api/debug/memory`, `/debug/beliefs`, `/api/debug/beliefs` and subroutes | `handlers_memory.go`, `handlers_belief_debug.go` |
| Metrics | `/api/metrics/tokens`, memory, traces, logs, log detail | `metrics_clickhouse.go`, `process_metrics.go`, `logs_clickhouse.go`, `traces_clickhouse.go` |
| Matrix/Pulse | `/api/matrix/rooms`, room detail | `handlers_matrix.go`, `matrix_*.go` |
| CodeQA | `/api/codeqa/runs`, `/api/codeqa/runs/{run_id}`, events | `handlers_codeqa.go` |
| Playground | `/api/v1/playground/*` | `internal/playground/*`, handler registration in `router.go` |
| Media/audio | `/audio/{filename}`, `/stt` | `handlers_media.go` |
| Config | `/api/config/agentd` | `handlers_config.go`, `config_settings.go` |

## Generated OpenAPI

The route catalog in `internal/apidocs/spec.go` is used by `cmd/openapi` and runtime docs handlers. The crawl found 77 catalog entries. When adding or changing routes, update:

1. `internal/agentd/router.go`;
2. the corresponding handler;
3. `internal/apidocs/spec.go`;
4. frontend API client/types if the UI uses it;
5. tests.

## Streaming Endpoints

- Chat endpoints can stream server-sent events.
- Flow run events can return JSON or SSE.
- CodeQA run events can return JSON or SSE.

Do not change stream event names or payload shapes without updating frontend parsers.

## Evidence

- `internal/agentd/router.go` route registrations.
- `internal/apidocs/spec.go` route catalog and OpenAPI generation.
- `docs/openapi.md` describes generated docs behavior.
- `web/agentd-ui/src/api/*` uses the route surface.
