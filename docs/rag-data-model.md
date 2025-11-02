Title: RAG data model, IDs, and ontology (Milestone 2)

Purpose
- Define unified IDs, nodes/edges, and DDL necessary to support chunk-centric ingestion and hybrid retrieval, while remaining safe and idempotent with no behavior changes yet.

Unified IDs
- Doc: doc:<namespace>:<slug|hash>
- Chunk: chunk:<doc-id>:<i>
- Entity: ent:<type>:<value>
  - Examples: ent:person:ada_lovelace, ent:tag:postgres
- ExternalRef: ref:<source>:<key>
- Source: src:<system>:<id> (optional lineage/source objects)

Node labels (graph)
- Doc: document-level node for provenance and metadata
- Chunk: chunk-level node aligned to the chunks table rows
- Entity: normalized entities, tags, topics, etc.
- ExternalRef: pointers to outside systems (URLs, IDs)
- Source: origin systems (crawler, uploader, application component)

Edges (graph)
- HAS_CHUNK: Doc -> Chunk (1-to-many)
- MENTIONS: Chunk -> Entity (N-to-N)
- REFERS_TO: Doc|Chunk -> ExternalRef (N-to-N)
- VERSION_OF: Doc -> Doc (version lineage)

Postgres tables and columns
- documents (existing; kept backward compatible)
  - Add columns (all IF NOT EXISTS):
    - title TEXT
    - url TEXT
    - lang regconfig DEFAULT 'english'
    - doc_hash TEXT
    - version INT DEFAULT 1
    - source TEXT
    - acl JSONB DEFAULT '{}'::jsonb
  - Indexes (all IF NOT EXISTS):
    - documents_doc_hash_uniq UNIQUE ON (doc_hash) WHERE doc_hash IS NOT NULL
    - documents_metadata_idx GIN(metadata)
    - documents_version_idx BTREE(version)
  - Note: existing ts tsvector is preserved as-is (current implementation uses the 'simple' config). Future work may migrate ts to use per-row lang.

- chunks (new)
  - id TEXT PRIMARY KEY
  - doc_id TEXT NOT NULL
  - idx INT NOT NULL
  - text TEXT NOT NULL
  - metadata JSONB NOT NULL DEFAULT '{}'::jsonb
  - lang regconfig DEFAULT 'english'
  - ts tsvector GENERATED ALWAYS AS (to_tsvector(lang, coalesce(text,''))) STORED
  - created_at TIMESTAMPTZ DEFAULT now()
  - updated_at TIMESTAMPTZ DEFAULT now()
  - Indexes (IF NOT EXISTS; created concurrently where supported):
    - chunks_ts_idx GIN(ts)
    - chunks_metadata_idx GIN(metadata)
    - chunks_doc_idx BTREE(doc_id, idx)

- embeddings (existing)
  - Add a vector ANN index if table/extension exist:
    - CREATE EXTENSION IF NOT EXISTS vector;
    - CREATE INDEX IF NOT EXISTS embeddings_vec_hnsw_cosine ON embeddings USING hnsw (vec vector_cosine_ops) WITH (m=16, ef_construction=64);
    - Note: choose IVFFLAT or HNSW depending on environment; HNSW is the default here.

- graph (existing: nodes, edges)
  - Ensure unique logical edges to avoid duplicates:
    - CREATE UNIQUE INDEX IF NOT EXISTS edges_uniq ON edges(source, rel, target);

Idempotency and safety
- All DDL uses IF NOT EXISTS and avoids destructive changes.
- Index creation statements are standalone and safe to re-run.
- No runtime behavior change is introduced in this milestone.

References
- See migrations under migrations/*.sql that implement the above DDL.

