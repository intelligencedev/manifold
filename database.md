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
  search:
    backend: memory   # memory | none | (future: opensearch, meilisearch, pg)
    dsn: ""           # optional connection string/URL
    index: docs       # optional index/collection name
  vector:
    backend: memory   # memory | none | (future: pgvector, qdrant, milvus)
    dsn: ""
    index: vectors
    dimensions: 1536  # optional; for validation in some backends
    metric: cosine    # optional; cosine | dot | l2 (backend‑specific)
  graph:
    backend: memory   # memory | none | (future: neo4j, memgraph, postgres)
    dsn: ""

Environment variables override YAML and can be used without a config file:

- SEARCH_BACKEND, SEARCH_DSN, SEARCH_INDEX
- VECTOR_BACKEND, VECTOR_DSN, VECTOR_INDEX, VECTOR_DIMENSIONS, VECTOR_METRIC
- GRAPH_BACKEND, GRAPH_DSN

Defaults

If not specified, all three backends default to memory. Set a backend to none to disable it (no‑op implementation).

Factory and dependency injection

Use the factory to construct backends from the loaded config:

mgr, err := databases.NewManager(ctx, cfg.Databases)
if err != nil { /* handle */ }
// mgr.Search (FullTextSearch), mgr.Vector (VectorStore), mgr.Graph (GraphDB)

Testing

Unit tests cover the in‑memory implementations and the configuration loader. See:

- internal/persistence/databases/databases_test.go
- internal/config/db_loader_test.go

Adding real backends

To add a new backend, implement the appropriate interface(s) in a new file or subpackage under internal/persistence/databases (e.g., opensearch_search.go, pgvector_vector.go, neo4j_graph.go), and extend NewManager to recognize a new Backend string (e.g., "opensearch", "pgvector", "neo4j"). Keep dependencies contained and follow the project’s package and DI guidelines.

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

These tools are implemented under internal/tools/db and automatically registered in both the CLI agent and the TUI. They sit on top of the database interfaces, so swapping backends requires no tool changes.

Design notes: how to use them effectively

- Use full‑text for rapid keyword search over raw documents, logs, or user notes.
- Use vector store for semantic similarity, e.g., finding related chunks of prior tasks or docs.
- Use the graph to model relationships: tasks ↔ files ↔ owners ↔ services, etc. Query neighbors to traverse relevant context quickly.

Typical flow example

1) While working, store summaries and artifacts:
   - search_index(id: "task-123:summary", text: "…summary…")
   - vector_upsert(id: "task-123:chunk-1", vector: [..], metadata: {task:"123"})
   - graph_upsert_node(id: "task-123", labels:["Task"], props:{title:"…"})
   - graph_upsert_edge(src:"task-123", rel:"TOUCHED", dst:"file:main.go")
2) Later, recall with search_query or vector_query; use graph_neighbors to expand context.
