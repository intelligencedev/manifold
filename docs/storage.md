# Storage

Manifold uses SQLite as the default durable backend and stores project files on
the local filesystem under the configured `WORKDIR`.

No `DATABASE_URL` is required for local or single-binary deployments.

## Durable Database

SQLite and Postgres DB-backed specialist/MCP secrets are encrypted at the
application layer. Set `MANIFOLD_SECRETS_KEY` in the process environment before
starting Manifold:

```bash
MANIFOLD_SECRETS_KEY=$(openssl rand -base64 32)
```

Generate this value once per deployment and keep it with your operational
secrets. The same key is required after backup/restore. If the key is lost,
encrypted database secrets cannot be recovered.

The default database configuration is:

```yaml
databases:
  backend: sqlite
  sqlite:
    path: "~/.manifold/manifold.db"
    busyTimeoutMs: 10000
    wal: true
    maxOpenConns: 1
```

SQLite stores chat history, search indexes, vector data, graph data, durable
tasks, preferences, auth data, and other local runtime state. Keyword search
uses SQLite FTS5. Vector search uses the bundled Vec1 extension through
`github.com/ncruces/go-sqlite3`, so cgo-free builds can still use vector
storage.

Run a local storage health check with:

```bash
manifold storage doctor --json
```

The doctor verifies SQLite open, WAL mode, FTS5, Vec1 registration through
`vec1_info()`, and a temporary vector insert/query.

## Layout

Each project is a directory at:

```text
$WORKDIR/users/<user-id>/projects/<project-id>
```

Metadata lives at:

```text
$WORKDIR/users/<user-id>/projects/<project-id>/.meta/project.json
```

## Configuration

Set `WORKDIR` in `.env` or in the process environment.

In the current example configuration, `WORKDIR` is provided through `.env` and does not need a matching `workdir:` field in `config.yaml` unless you explicitly want YAML to supply it.

No Postgres storage backend configuration is required for local deployments.

## Optional Postgres

Postgres remains supported for deployments that need multi-host concurrency or
existing Postgres data. Configure it explicitly:

```yaml
databases:
  backend: postgres
  defaultDSN: "${DATABASE_URL}"
  chat:
    backend: postgres
    dsn: "${DATABASE_URL}"
  search:
    backend: postgres
    dsn: "${DATABASE_URL}"
  vector:
    backend: postgres
    dsn: "${DATABASE_URL}"
  graph:
    backend: postgres
    dsn: "${DATABASE_URL}"
```

Automatic Postgres-to-SQLite migration is not part of this release. Existing
Postgres users keep Postgres unless they opt into SQLite.

## Operational Notes

- Deleting a project removes the directory immediately.
- Files are read and written directly on disk.
- All file paths are validated to stay inside the project directory.
- SQLite is intended for single-process local deployments. Use Postgres for
  multi-process, multi-host deployments.
