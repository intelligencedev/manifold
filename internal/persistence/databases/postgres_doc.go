package databases

/*
Extensions
- vector: for pgvector (embeddings)
- postgis: required by many routing functions; optional here but enabled if possible
- pgrouting: for shortest paths (not used directly yet)
- pg_trgm: optional FTS helpers (not required for tsquery)

Tables
- documents(id TEXT PRIMARY KEY, text TEXT NOT NULL, metadata JSONB, ts tsvector GENERATED ... STORED)
  GIN index on ts
- embeddings(id TEXT PRIMARY KEY, vec vector[(dim)], metadata JSONB)
  Consider ivfflat index per metric
- nodes(id TEXT PRIMARY KEY, labels TEXT[], props JSONB)
- edges(id BIGSERIAL PK, source TEXT, rel TEXT, target TEXT, props JSONB)
  Indexes on (source, rel) and (target, rel)
*/
