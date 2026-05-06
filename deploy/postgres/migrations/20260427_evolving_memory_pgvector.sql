-- Add pgvector-backed retrieval support for evolving memory.
-- HNSW indexes require a dimensioned pgvector column. This matches the default
-- embedding dimensions configured for Manifold.

CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS embedding_vec vector(1536);

ALTER TABLE evolving_memories
    ALTER COLUMN embedding_vec TYPE vector(1536) USING embedding_vec::vector(1536);

CREATE INDEX IF NOT EXISTS evolving_memories_embedding_hnsw_idx
    ON evolving_memories USING hnsw (embedding_vec vector_cosine_ops)
    WHERE embedding_vec IS NOT NULL;
