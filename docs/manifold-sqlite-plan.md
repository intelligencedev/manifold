# Default SQLite Storage With Vec1, Optional Postgres

## Summary
Make SQLite the default durable backend so Manifold can run as a single binary without PostgreSQL. Keep existing Postgres paths as an explicit optional backend for deployments that need multi-host concurrency or existing Postgres data.

Use `github.com/ncruces/go-sqlite3` with `github.com/ncruces/go-sqlite3/ext/vec1` registered on every connection, because it is cgo-free, works with `CGO_ENABLED=0` builds, supports `database/sql`, and exposes a Vec1 registration hook. Use SQLite FTS5 for keyword search and Vec1 for vector search.

## Public Config And Interfaces
- Add root database selection:
  - `databases.backend: sqlite | postgres`, default `sqlite`.
  - `databases.sqlite.path`, default `~/.manifold/manifold.db`.
  - `databases.sqlite.busyTimeoutMs: 10000`, `wal: true`, `maxOpenConns: 1`.
- Extend existing `chat`, `search`, `vector`, and `graph` backend values to include `sqlite`; keep `postgres`, `memory`, `auto`, `none` behavior.
- Keep existing store interfaces unchanged. Add SQLite implementations behind the current `Manager` fields rather than changing callers.
- Keep `databases.defaultDSN` for Postgres. It is not required for the SQLite default path.
- Postgres remains available only through a configured external endpoint.

## Key Implementation Changes
- Replace the placeholder `internal/persistence/sqlite` package with a shared SQLite opener, migration runner, and helpers for JSON, timestamps, transactions, and Vec1 registration using `driver.Open(dsn, vec1.Register)`.
- Add SQLite implementations for all currently Postgres-backed durable stores: chat, specialist activity, Flow v2, durable tasks, MCP config, projects metadata, user preferences, pulse, Matrix messages, transit, belief memory, evolving memory, playground, auth, and CodeQA.
- Use SQLite schemas with `INTEGER PRIMARY KEY`, `TEXT` IDs, UTC RFC3339 timestamps, boolean `INTEGER`, JSON stored as validated text, and a `sqlite_schema_migrations` table.
- Implement FTS5-backed search with content tables plus FTS virtual tables for documents and chunks; use `MATCH`, `bm25`, and `snippet`/`highlight` equivalents.
- Implement graph storage with SQLite adjacency tables plus MAGMA event/entity/typed-edge tables, preserving the current graph interfaces and maintenance behavior.
- Implement Vec1 vector storage with:
  - Vec1 virtual table storing native float32 vector BLOBs.
  - Side metadata table mapping internal rowid to Manifold string IDs and JSON metadata.
  - Fixed promoted metadata columns for common filters such as `tenant`, `type`, and `doc_id`.
  - `cosine` and `l2` support; reject SQLite `ip/dot` until Vec1 supports dot product.
  - Vec1 ANN enabled by default after `minRows: 5000`; before that, use Vec1 flat/exact mode.
  - Async ANN rebuild when row count crosses the threshold or changed rows exceed 1000.
  - Query overfetch plus exact reranking with `vec1_cos_distance` or `vec1_l2_distance`.
- Rework durable task claiming for SQLite using `BEGIN IMMEDIATE` transactions and atomic `UPDATE ... RETURNING` patterns instead of Postgres `FOR UPDATE SKIP LOCKED`. SQLite support is single-process/multi-goroutine; Postgres remains the multi-process/multi-host backend.

## Docs And Rollout
- Update `config.yaml.example`, `.env` docs, README, QUICKSTART, and deployment docs so first-run local setup has no `DATABASE_URL` requirement.
- Document optional Postgres configuration separately and keep existing Postgres examples valid.
- Add `manifold storage doctor --json` to verify SQLite open, FTS5 availability, Vec1 registration via `vec1_info()`, WAL mode, and a temp vector insert/query.
- Do not implement automatic Postgres-to-SQLite migration in v1. Existing users keep Postgres unless they opt into SQLite.

## Test Plan
- Add SQLite parity tests for every store interface using temp database files.
- Add config tests for default SQLite, explicit Postgres compatibility, and invalid vector metrics.
- Add FTS5 tests for indexing, chunk search, snippets, delete/update, and metadata filters.
- Add Vec1 tests for registration, cosine/l2 search, metadata filtering, ANN rebuild state, and exact reranking.
- Add durable concurrency tests for claim/heartbeat/complete/cancel/retry under parallel goroutines.
- Run `go test ./...`, `go test -race ./...`, `make fmt-check`, `make imports-check`, `go vet ./...`, and `make lint`.
- Build smoke: `CGO_ENABLED=0 go build ./cmd/...` plus the existing cross-build path.

## Assumptions
- SQLite is the default local backend; Postgres remains supported and recommended for multi-host deployments.
- Vec1 is bundled through the Go dependency, not loaded from a shared library.
- No automatic data migration from existing Postgres databases in the first SQLite release.
- Primary references: [SQLite Vec1](https://sqlite.org/vec1), [Vec1 user manual](https://sqlite.org/vec1/doc/trunk/doc/vec1intro.md), [SQLite FTS5](https://www.sqlite.org/fts5.html), [ncruces driver](https://pkg.go.dev/github.com/ncruces/go-sqlite3/driver), [ncruces Vec1 package](https://pkg.go.dev/github.com/ncruces/go-sqlite3/ext/vec1).
