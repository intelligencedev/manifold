# Data: Postgres Schema

This page summarizes tables created by application store initialization and migrations. It is a map, not a complete SQL reference.

## Schema Groups

| Group | Tables |
| --- | --- |
| Auth | `users`, `roles`, `user_roles`, `sessions` |
| Chat | `chat_sessions`, `chat_messages`, `chat_agent_activities` |
| Projects | `projects`, `project_files` |
| Specialists/teams | `specialists`, `specialist_groups`, `specialist_group_memberships` |
| MCP | `mcp_servers` |
| Preferences | `user_preferences` |
| Flow v2 | `flow_v2_workflows` |
| Pulse/Matrix | `pulse_rooms`, `pulse_tasks`, `matrix_messages` |
| Transit | `transit_memories` |
| Evolving memory | `evolving_memories` |
| Belief memory | `belief_scopes`, `belief_episodes`, `beliefs`, `belief_evidence`, `belief_promotions` |
| Search/vector/graph | `documents`, `embeddings`, `nodes`, `edges` |
| Playground | `playground_prompts`, `playground_prompt_versions`, `playground_datasets`, `playground_snapshots`, `playground_rows`, `playground_experiments`, `playground_runs`, `playground_run_results` |
| CodeQA | `codeqa_runs`, `codeqa_run_events` |
| Policy/reactive claims | `reactive_claims` |

## ER Overview

```mermaid
erDiagram
    USERS ||--o{ SESSIONS : owns
    USERS ||--o{ USER_ROLES : has
    ROLES ||--o{ USER_ROLES : grants

    CHAT_SESSIONS ||--o{ CHAT_MESSAGES : contains
    CHAT_SESSIONS ||--o{ CHAT_AGENT_ACTIVITIES : has

    SPECIALIST_GROUPS ||--o{ SPECIALIST_GROUP_MEMBERSHIPS : has
    SPECIALISTS ||--o{ SPECIALIST_GROUP_MEMBERSHIPS : belongs_to

    PULSE_ROOMS ||--o{ PULSE_TASKS : schedules

    BELIEF_SCOPES ||--o{ BELIEF_EPISODES : contains
    BELIEF_SCOPES ||--o{ BELIEFS : contains
    BELIEFS ||--o{ BELIEF_EVIDENCE : has
    BELIEFS ||--o{ BELIEF_PROMOTIONS : has

    PLAYGROUND_DATASETS ||--o{ PLAYGROUND_SNAPSHOTS : versions
    PLAYGROUND_SNAPSHOTS ||--o{ PLAYGROUND_ROWS : contains
    PLAYGROUND_EXPERIMENTS ||--o{ PLAYGROUND_RUNS : has
    PLAYGROUND_RUNS ||--o{ PLAYGROUND_RUN_RESULTS : has

    CODEQA_RUNS ||--o{ CODEQA_RUN_EVENTS : emits
```

## Schema Change Checklist

- Update application `Init` SQL.
- Add or update deploy migration when appropriate.
- Update store tests and tenant/user scoping tests.
- Update this page and [Evidence](../evidence.md).
- Check OpenAPI and frontend types if the schema affects API payloads.

## Evidence

- SQL extraction from `internal/persistence/databases/*_postgres*.go`.
- `internal/auth/store.go` auth schema.
- `internal/codeqa/store/postgres.go` CodeQA schema.
- `deploy/postgres/migrations/*` migration files.
