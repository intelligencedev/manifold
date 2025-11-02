RAG repository discovery — Search, Vector, Graph backends and wiring

Scope
- Goal: document the existing Postgres-backed search, vector, and graph implementations, their public interfaces, initialization/wiring, and schemas so we can add rag_ingest and rag_retrieve without breaking behavior.
- This is a read-only alignment doc; no code changes in this milestone.

Key packages and files
- internal/persistence/databases
  - interfaces.go — public interfaces and Manager
  - postgres_search.go — Postgres FTS impl and bootstrap
  - postgres_vector.go — Postgres pgvector impl and bootstrap
  - postgres_graph.go — Postgres graph tables impl and bootstrap
  - postgres_doc.go — inline documentation of expected extensions/schemas
  - memory_search.go, memory_vector.go, memory_graph.go — in-memory backends
  - factory.go — Manager factory that selects and initializes backends
- internal/tools/db — helper tools that orchestrate search+vector+graph
  - index_document.go — dual-index (FTS + vector) ingestion helper
  - hybrid.go — hybrid retrieval (FTS + vector) with score fusion
  - search.go, vector.go, graph.go — tool wrappers around interfaces
- internal/config/config.go — configuration types (DBConfig, Embedding, etc.)

Public interfaces and constructors

FullTextSearch (internal/persistence/databases/interfaces.go)
- Methods
  - Index(ctx, id, text, metadata) error
  - Remove(ctx, id) error
  - Search(ctx, query, limit) ([]SearchResult, error)
  - GetByID(ctx, id) (SearchResult, bool, error)
- Types
  - SearchResult { ID string; Score float64; Snippet string; Text string; Metadata map[string]string }
- Implementations / constructors
  - NewMemorySearch() FullTextSearch
  - NewPostgresSearch(pool *pgxpool.Pool) FullTextSearch
    - Best-effort bootstrap (extensions, tables, indexes). See “Postgres schemas”.

VectorStore (interfaces.go)
- Methods
  - Upsert(ctx, id, vector, metadata) error
  - Delete(ctx, id) error
  - SimilaritySearch(ctx, vector, k, filter) ([]VectorResult, error)
- Types
  - VectorResult { ID string; Score float64; Metadata map[string]string }
- Implementations / constructors
  - NewMemoryVector() VectorStore
  - NewPostgresVector(pool *pgxpool.Pool, dimensions int, metric string) VectorStore
    - Best-effort bootstrap and metric-aware similarity. See “Postgres schemas”.
    - Exposes Dimension() int on the concrete type for validation (via type assertion).

GraphDB (interfaces.go)
- Methods
  - UpsertNode(ctx, id, labels []string, props map[string]any) error
  - UpsertEdge(ctx, srcID, rel, dstID string, props map[string]any) error
  - Neighbors(ctx, id, rel) ([]string, error)
  - GetNode(ctx, id) (Node, bool)
- Types
  - Node { ID string; Labels []string; Props map[string]any }
- Implementations / constructors
  - NewMemoryGraph() GraphDB
  - NewPostgresGraph(pool *pgxpool.Pool) GraphDB
    - Best-effort bootstrap (extensions, tables, indexes). See “Postgres schemas”.

Manager / factory wiring
- Type: Manager { Search FullTextSearch; Vector VectorStore; Graph GraphDB; Chat persistence.ChatStore; Playground *PlaygroundStore; Warpp persistence.WarppWorkflowStore }
- Factory: NewManager(ctx context.Context, cfg config.DBConfig) (Manager, error)
  - Resolves per-subsystem backends based on cfg.Search/Vector/Graph.Backend with DSN fallbacks to cfg.DefaultDSN.
  - Supported backend values: "memory" (default), "auto", "postgres" (aliases: pg, pgvector for vector), and "none"/"disabled".
  - "auto": if DSN present and Postgres reachable, chooses Postgres; else falls back to memory.
  - For "postgres", DSN is required; newPgPool creates a pgxpool with conservative defaults and verifies connectivity (Ping with 3s timeout).
  - Also wires Chat and optional Warpp store when DefaultDSN is set (not directly relevant to RAG service).

Postgres schemas and initialization

Full-text search (postgres_search.go)
- Extensions
  - CREATE EXTENSION IF NOT EXISTS pg_trgm (best-effort; non-fatal)
- Tables
  - documents (
      id TEXT PRIMARY KEY,
      text TEXT NOT NULL,
      metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
      ts tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce(text,''))) STORED
    )
  - Index: CREATE INDEX IF NOT EXISTS documents_ts_idx ON documents USING GIN (ts)
- Query behavior
  - Search: plainto_tsquery('simple', $query) over ts; ORDER BY ts_rank DESC; LIMIT $limit
  - GetByID: SELECT id, text, metadata FROM documents WHERE id=$1
  - Snippet: left(text, 120) (no ts_headline; language config is ‘simple’ by default)
- Notes / constraints
  - metadata is JSONB NOT NULL with default {}; Index/Update uses upsert semantics by id.
  - Language-aware stemming is not configured yet (uses 'simple').

Vector (postgres_vector.go)
- Extensions
  - CREATE EXTENSION IF NOT EXISTS vector
- Tables
  - embeddings (
      id TEXT PRIMARY KEY,
      vec vector[(dimensions)],
      metadata JSONB NOT NULL DEFAULT '{}'::jsonb
    )
  - No index is created by default (comment suggests DBA/tuning: ivfflat per metric).
- Similarity search
  - Metric is normalized to lowercase; supported: cosine (default), l2/euclidean, ip/dot.
  - Operators and score normalization:
    - cosine: ORDER BY vec <=> $1::vector; score = 1 - distance
    - l2: ORDER BY vec <-> $1::vector; score = -distance (higher is better)
    - ip: ORDER BY vec <#> $1::vector; score = -distance (higher is better)
  - Optional filter via WHERE metadata @> $filter (exact containment on JSONB)
- Upsert
  - INSERT ... ON CONFLICT(id) DO UPDATE SET vec=EXCLUDED.vec, metadata=EXCLUDED.metadata
- Notes / constraints
  - metadata is JSONB NOT NULL with default {}.
  - Dimensions: if >0, table is created as vector(dimensions); else generic vector.

Graph (postgres_graph.go)
- Extensions (best-effort, not required for basic ops)
  - CREATE EXTENSION IF NOT EXISTS postgis
  - CREATE EXTENSION IF NOT EXISTS pgrouting
- Tables
  - nodes (
      id TEXT PRIMARY KEY,
      labels TEXT[] NOT NULL DEFAULT '{}',
      props JSONB NOT NULL DEFAULT '{}'::jsonb
    )
  - edges (
      id BIGSERIAL PRIMARY KEY,
      source TEXT NOT NULL,
      rel TEXT NOT NULL,
      target TEXT NOT NULL,
      props JSONB NOT NULL DEFAULT '{}'::jsonb
    )
  - Indexes: edges_src_rel on (source, rel); edges_dst_rel on (target, rel)
- Behavior
  - UpsertNode: INSERT ... ON CONFLICT(id) DO UPDATE labels/props
  - UpsertEdge: INSERT ... ON CONFLICT DO NOTHING (no unique constraint → no dedupe; duplicates possible)
  - Neighbors: SELECT target FROM edges WHERE source=$1 AND rel=$2 ORDER BY target
  - GetNode: SELECT labels, props FROM nodes WHERE id=$1 (returns (Node,false) if not found; no error)
- Notes / constraints
  - props is JSONB NOT NULL with default {}, same for edges.props. No referential integrity between nodes and edges is enforced.

Higher-level orchestration patterns to reuse
- internal/tools/db/index_document.go
  - Provides a single tool that indexes into both FTS and vector stores.
  - Single-document: requires id and text → Search.Index then VectorUpsert (embeddings if vector not provided).
  - Batch: supports texts or texts_json (either []string or {"chunks": []string}); IDs formed as id_prefix+":"+i; configurable concurrency (default 4; max 64) using errgroup.
  - Embeddings via internal/embedding.EmbedText using config.EmbeddingConfig.
- internal/tools/db/hybrid.go
  - Performs hybrid retrieval across FTS and vector stores.
  - Input options: query (for FTS), text or vector (for embedding/vector query), k, filter, alpha/beta (default 0.4/0.6).
  - Normalizes per-modality scores by max; fuses with weighted sum; deduplicates by ID; returns top-k.
- internal/tools/db/search.go, vector.go, graph.go
  - Thin wrappers exposing the core interfaces as tools; show how to structure arguments and validate dimensions for pgvector via Dimension() type assertion.

Initialization and import patterns for internal/rag/service
- Import paths
  - Backends and interfaces: "manifold/internal/persistence/databases"
  - Config types: "manifold/internal/config"
  - Optional embedding: "manifold/internal/embedding"
- Recommended initialization
  - Load top-level config.Config (or DBConfig directly) via existing loader patterns, then construct a Manager:
    - m, err := databases.NewManager(ctx, cfg.Databases)
    - Use m.Search (FullTextSearch), m.Vector (VectorStore), m.Graph (GraphDB)
  - Alternatively, construct directly for fine-grained control:
    - pool, _ := pgxpool.NewWithConfig(...); s := databases.NewPostgresSearch(pool)
    - v := databases.NewPostgresVector(pool, cfg.Databases.Vector.Dimensions, cfg.Databases.Vector.Metric)
    - g := databases.NewPostgresGraph(pool)
  - The Postgres constructors perform best-effort bootstrap (CREATE EXTENSION/TABLE/INDEX IF NOT EXISTS) suitable for dev; production should use migrations.
- Surface alignment
  - Ingestion service (rag_ingest) can emulate internal/tools/db/index_document.go semantics: idempotent upsert into FTS and vector, optional embedding, batch+concurrency, unified IDs.
  - Retrieval service (rag_retrieve) can reuse the hybrid approach in internal/tools/db/hybrid.go and optionally add reranking/graph expansion. Both FTS and vector results carry metadata maps for future ACL filters.

Config points and feature flags affecting behavior
- DB backends (internal/config/config.go → DBConfig)
  - Databases.DefaultDSN: shared DSN fallback when per-subsystem DSN missing.
  - Search.Backend/DSN/Index: backend selector; Index is currently unused in code.
  - Vector.Backend/DSN/Index/Dimensions/Metric: pgvector dimensions/metric influence schema and similarity operator and score expression.
  - Graph.Backend/DSN: selects memory/postgres/auto; DSN required for postgres.
  - Backends accept values: "memory" (default), "auto", "postgres" (alias "pg"; vector also "pgvector"), "none"/"disabled" (no-op implementations).
- Embeddings (EmbeddingConfig)
  - BaseURL, Path, APIKey, APIHeader, Model, Timeout drive embedding generation for ingestion (vector upsert from text) and hybrid retrieval when only text query is provided.
- Observations for future ACL/language support
  - FTS currently uses the 'simple' text search configuration. No per-language stemming; enhancing language-aware FTS would require query-time config or per-row tsvector columns per language.
  - Vector search supports metadata JSONB filtering (metadata @> filter), but FTS Search does not currently accept a filter; consistent ACL filtering would require extending the FTS interface and implementation to filter on metadata.
  - Graph stores arbitrary props with JSONB; no ACL enforcement at this layer.

Constraints and compatibility notes
- Postgres constructors perform best-effort CREATE EXTENSION/TABLE/INDEX and ignore errors where appropriate; this is non-fatal for non-superuser environments but means capabilities may vary (e.g., no pg_trgm, no postgis/pgrouting).
- Graph edges: ON CONFLICT DO NOTHING without a unique constraint means duplicates are not prevented; deduplication must be handled at a higher layer if needed.
- Vector similarity normalizes to higher-is-better scores for fusion, but absolute scales differ by metric; hybrid fusion code already normalizes by max per modality.
- All metadata maps are JSONB NOT NULL with default {}; callers should provide an empty map instead of nil (the code guards against nil by using defaults in Postgres backends).

How to instantiate within internal/rag/service
- Use the Manager for consistent wiring and future reuse of configuration/feature flags:
  - m, err := databases.NewManager(ctx, cfg.Databases)
  - s := m.Search; v := m.Vector; g := m.Graph
- For ingestion: follow index_document.go as reference for chunk-centric ID strategy (id_prefix:index), concurrency, and embedding.
- For retrieval: follow hybrid.go for fusion, and layer optional reranking; plan to add ACL filtering and graph augmentation using g.Neighbors/GetNode.

Summary
- The current repository exposes stable interfaces and constructors for FTS, vector, and graph backends with a Manager to wire them from config. Postgres implementations bootstrap minimal schemas:
  - FTS: documents(id, text, metadata, ts) with GIN(ts) and 'simple' config.
  - Vector: embeddings(id, vec, metadata) with pgvector, metric-aware ops.
  - Graph: nodes(id, labels, props), edges(id, source, rel, target, props) with indexes on (source, rel) and (target, rel).
- We can implement rag_ingest and rag_retrieve by reusing these interfaces, the Manager, and the patterns in internal/tools/db to provide idempotent chunk ingestion and hybrid retrieval, adding ACL/reranking/graph augmentation at the service layer without changing existing behavior.

