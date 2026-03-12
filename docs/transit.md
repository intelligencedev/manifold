# Transit Shared Memory

Transit is Manifold's durable shared-memory layer for agents. It stores addressable records under hierarchical keys so specialists and delegated agents can coordinate through explicit shared state instead of only through chat history.

## Current Scope

This first implementation ships the owner-scoped MVP:

- durable records stored in Postgres when `databases.defaultDSN` is configured
- in-memory fallback for local development and tests
- create, get, update, delete
- prefix-based key listing and recent-key listing
- search and metadata-only discovery
- internal tool exposure and authenticated HTTP endpoints

Not included yet:

- namespace ACL grants beyond owner scope
- subscriptions and event streaming
- public namespaces
- cross-org federation
- hypergraph or other derived-memory jobs

## Record Model

Each Transit record is scoped to a tenant and keyed by `keyName`.

```json
{
  "keyName": "project/demo/brief",
  "description": "Shared project brief",
  "value": "Durable coordination note for the team",
  "base64": false,
  "embed": true,
  "embedSource": "value"
}
```

Stored fields include:

- `tenantId`
- `keyName`
- `description`
- `value`
- `base64`
- `embed`
- `embedSource`
- `version`
- `createdBy` / `updatedBy`
- `createdAt` / `updatedAt`

`keyName` must be hierarchical and stable. Allowed characters are letters, numbers, `_`, `.`, `/`, `@`, and `-`. Keys cannot contain `..` or start/end with `/`.

## Configuration

Enable Transit in `config.yaml`:

```yaml
enableTools: true

transit:
  enabled: true
  defaultSearchLimit: 10
  defaultListLimit: 100
  maxBatchSize: 100
  enableVectorSearch: true
```

If you use a global tool allow-list, include the Transit tool names there as well:

```yaml
allowTools:
  - transit_create
  - transit_get
  - transit_update
  - transit_delete
  - transit_search
  - transit_discover
  - transit_list_keys
  - transit_list_recent
```

Environment variable overrides:

```bash
ENABLE_TOOLS=true
TRANSIT_ENABLED=true
TRANSIT_DEFAULT_SEARCH_LIMIT=10
TRANSIT_DEFAULT_LIST_LIMIT=100
TRANSIT_MAX_BATCH_SIZE=100
TRANSIT_ENABLE_VECTOR_SEARCH=true
```

Persistence uses the shared `databases.defaultDSN` setting. If no Postgres DSN is configured, Transit falls back to an in-memory store.

Search uses the configured search backend. Vector search uses the configured vector backend plus the existing embedding service.

Transit registration is gated at startup. The Transit tools are only registered when:

- `transit.enabled` is `true`
- the server starts successfully with a Transit store
- tools are globally enabled with `enableTools: true` or `ENABLE_TOOLS=true`

If `allowTools` or `ALLOW_TOOLS` is set, only the listed tools are exposed to agents. In that case, Transit APIs still work, but Transit tools will not appear in the tool catalog unless the `transit_*` tool names are included.

## Tool Surface

When Transit is enabled, Manifold registers these internal tools:

- `transit_create`
- `transit_get`
- `transit_update`
- `transit_delete`
- `transit_search`
- `transit_discover`
- `transit_list_keys`
- `transit_list_recent`

`transit_search` returns full records. `transit_discover` returns metadata-only hits to reduce context size.

## Verification

After enabling Transit and restarting `agentd`, verify it in this order:

1. Confirm the server started with the updated config.
2. Open the tools catalog and check that the `transit_*` tools are present.
3. Call one of the Transit HTTP endpoints such as `POST /api/transit/memories`.

If the tools are missing, the most common causes are:

- `transit.enabled` is still `false`
- `enableTools` is `false`
- `ALLOW_TOOLS` or `allowTools` excludes the Transit tool names
- the server was not restarted after changing config

## HTTP API

Authenticated endpoints:

- `POST /api/transit/memories`
- `GET /api/transit/memories?keys=a,b`
- `DELETE /api/transit/memories?keys=a,b`
- `PUT /api/transit/memories/{key}`
- `GET /api/transit/keys?prefix=project/demo`
- `GET /api/transit/recent?prefix=project/demo`
- `POST /api/transit/search`
- `POST /api/transit/discover`

Example create request:

```json
{
  "items": [
    {
      "keyName": "project/demo/brief",
      "description": "Shared project brief",
      "value": "Transit stores durable shared project memory"
    }
  ]
}
```

Example update request:

```json
{
  "value": "Updated shared project memory",
  "ifVersion": 1
}
```

## Search Behavior

Transit writes records to the authoritative Transit store and best-effort indexes them into:

- full-text search for keyword retrieval
- vector search for semantic retrieval when enabled

Search merges index hits with authoritative rows. If an index is unavailable or incomplete, Transit falls back to direct store search.

Current consistency model:

- authoritative writes are synchronous
- search indexing is attempted inline during create/update/delete
- if indexing fails, the authoritative row remains the source of truth

## Operational Notes

- The initial tenant boundary is the current Manifold user. When auth is disabled, Transit uses the system tenant.
- The current implementation is owner-scoped only. Multi-user grants are not enforced yet.
- `version` supports optimistic concurrency for updates through `ifVersion`.
- `embedSource=description` is useful when descriptions are stable and values change frequently.

## Next Work

The next Transit phases should add:

1. additive namespace ACLs
2. subscriptions and replayable event delivery
3. public read namespaces
4. derived-memory jobs
5. cross-tenant sharing and federation