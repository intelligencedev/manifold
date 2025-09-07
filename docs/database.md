Database backends (search, vectors, graph)

This project defines small, idiomatic Go interfaces to allow swapping different database backends for:

- Full‑text search (FTS)
- Vector embeddings / nearest‑neighbor search
- Graph databases

By default, in‑memory implementations are used to make local development and testing easy. You can change each backend independently via config.yaml or environment variables.

Interfaces

The interfaces live under internal/persistence/databases and represent the minimal surface area we need:

- FullTextSearch: Index, Remove, Search
- VectorStore: Upsert, Delete, SimilaritySearch
- GraphDB: UpsertNode, UpsertEdge, Neighbors, GetNode

A Manager struct groups the three concrete backends so you can inject a single dependency where needed.

Configuration

Add a databases section to config.yaml (the same file already used by the app):

databases:
  # Optional shared DSN used by backends when their DSN is empty.
  # If set, and backend is left empty, the loader will set backend: auto
  # so the factory attempts Postgres, else falls back to memory.
  # defaultDSN: postgresql://user:pass@localhost:5432/app?sslmode=disable
  search:
    backend: memory   # auto | memory | none | postgres
    dsn: ""           # optional connection string/URL
  vector:
    backend: memory   # auto | memory | none | postgres(pgvector)
    dsn: ""
    dimensions: 1536  # optional; for validation in some backends
    metric: cosine    # optional; cosine | dot | l2 (backend‑specific)
  graph:
    backend: memory   # auto | memory | none | postgres
    dsn: ""

Environment variables override YAML and can be used without a config file:

- DATABASE_URL (shared default DSN)
- SEARCH_BACKEND, SEARCH_DSN, SEARCH_INDEX
- VECTOR_BACKEND, VECTOR_DSN, VECTOR_INDEX, VECTOR_DIMENSIONS, VECTOR_METRIC
- GRAPH_BACKEND, GRAPH_DSN

Defaults and auto selection

- If databases.defaultDSN or DATABASE_URL is set and a backend is left empty,
  the loader sets backend: auto. On startup the factory will:
  - try to connect to Postgres (pgxpool) using the resolved DSN,
  - if it succeeds, use Postgres-backed implementations (fts/vector/graph),
  - otherwise fall back to memory implementations.
- If backend is explicitly memory or none, that is honored regardless of DSN.

Postgres implementations

- FTS (internal/persistence/databases/postgres_search.go)
  - tables: documents (id, text, metadata, ts tsvector)
  - query with plainto_tsquery('simple', ...), order by ts_rank
- Vector (internal/persistence/databases/postgres_vector.go)
  - tables: embeddings (id, vec vector[(dim)], metadata)
  - distance ops: <=> (cosine), <-> (L2), <#> (inner product)
- Graph (internal/persistence/databases/postgres_graph.go)
  - tables: nodes, edges; basic neighbors and get node
- Best-effort CREATE EXTENSION IF NOT EXISTS for vector, postgis, pgrouting, pg_trgm.
  For production, prefer migrations and proper roles.

Factory and dependency injection

Use the factory to construct backends from the loaded config:

mgr, err := databases.NewManager(ctx, cfg.Databases)
if err != nil { /* handle */ }
// mgr.Search (FullTextSearch), mgr.Vector (VectorStore), mgr.Graph (GraphDB)

defer mgr.Close() // closes Postgres pools when present

Testing

Unit tests cover the in‑memory implementations and the configuration loader. See:

- internal/persistence/databases/databases_test.go
- internal/config/db_loader_test.go

Integration tests (future): use testcontainers to launch Postgres with pgvector + pgrouting and validate CRUD and queries.

Agent tools (how the assistant uses databases)

The assistant can use dedicated tools to store and retrieve information during tasks, boosting recall and success probability:

- Full‑text search tools
  - search_index(id, text, metadata?)
  - search_query(query, limit?) -> results
  - search_remove(id)
- Vector store tools
  - vector_upsert(id, vector, metadata?)
  - vector_query(vector, k?, filter?) -> results
  - vector_delete(id)
- Graph tools
  - graph_upsert_node(id, labels?, props?)
  - graph_upsert_edge(src, rel, dst, props?)
  - graph_neighbors(id, rel) -> neighbor IDs
  - graph_get_node(id) -> node

Design notes

- Use full‑text for rapid keyword search over raw documents, logs, or user notes.
- Use vector store for semantic similarity; choose metric per model.
- Use the graph to model relationships; neighbors to traverse.
