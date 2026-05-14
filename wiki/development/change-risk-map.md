# Change Risk Map

This page highlights high-risk areas for new contributors.

## High-Risk Areas

| Area | Why risky | Files to inspect |
| --- | --- | --- |
| Path sandboxing | Mistakes can expose files outside project workspaces or unsafe command contexts. | `internal/projects/service.go`, `internal/sandbox/*`, `internal/tools/filetool/tool.go`, `internal/tools/cli/exec.go`, `internal/mcpclient/pool.go` |
| Tool registry and schemas | Changes alter model capabilities and can break specialist/tool workflows. | `internal/tools/**`, `internal/agentd/app_init.go`, `internal/tools/discovery/*` |
| Chat streaming | Frontend depends on event names and payloads. | `internal/agentd/chat_execution.go`, `chat_transport.go`, `web/agentd-ui/src/api/chat.ts` |
| Agent loop | Affects all long-horizon reasoning, tools, summaries, and delegation. | `internal/agent/engine*.go` |
| Persistence schemas | App init and migrations must not diverge. | `internal/persistence/databases/*`, `deploy/postgres/migrations/*` |
| Auth scoping | Multi-user data isolation can regress silently. | `internal/auth/*`, handlers in `internal/agentd`, stores with `user_id`/tenant IDs |
| Memory prompt context | Retrieved content can become prompt injection if mislabeled. | `internal/rag/**`, `internal/transit/**`, `internal/agent/memory/**`, `internal/agent/belief/**` |
| MCP path-dependent servers | Server lifecycle depends on active project and auth mode. | `internal/mcpclient/pool.go`, `handlers_mcp.go` |
| Flow v2 runtime | Parallel execution, events, retries, and guards interact. | `internal/flow/*`, `internal/agentd/flow_v2_runtime.go` |
| Matrix/Pulse | Scheduled tasks can duplicate or spam rooms if leases/routing fail. | `internal/matrixgw/*`, `internal/pulse/service.go`, `internal/agentd/matrix_pulse.go` |
| CodeQA auto-apply | Can modify code or produce misleading results if gates/judges are misconfigured. | `internal/codeqa/**`, `internal/tools/codeqa/*` |

## Review Checklist

- [ ] Does the change preserve project/workspace containment?
- [ ] Does it respect user/tenant scoping?
- [ ] Are tool schemas and OpenAPI docs updated if contracts changed?
- [ ] Are stream events backward-compatible?
- [ ] Are migrations/store init/tests aligned?
- [ ] Are long-running operations cancellable?
- [ ] Are secrets redacted in logs and outputs?

## Evidence

Risk areas are derived from handler responsibilities, tool registration, store schemas, path policies, and tests under the corresponding packages.
