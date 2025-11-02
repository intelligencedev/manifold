-- Milestone 2: vector ANN index (pgvector) and graph edges uniqueness
-- Idempotent, safe to run multiple times.

-- pgvector extension (safe if already installed)
CREATE EXTENSION IF NOT EXISTS vector;

-- embeddings: ANN index (prefer HNSW; if vector_cosine_ops not available, adjust in future migrations)
DO $$
BEGIN
  IF to_regclass('public.embeddings') IS NOT NULL THEN
    IF to_regclass('public.embeddings_vec_hnsw_cosine') IS NULL THEN
      EXECUTE 'CREATE INDEX embeddings_vec_hnsw_cosine ON embeddings USING hnsw (vec vector_cosine_ops) WITH (m=16, ef_construction=64)';
    END IF;
  END IF;
END$$;

-- Graph edges: uniqueness to prevent duplicates
DO $$
BEGIN
  IF to_regclass('public.edges') IS NOT NULL THEN
    IF to_regclass('public.edges_uniq') IS NULL THEN
      EXECUTE 'CREATE UNIQUE INDEX edges_uniq ON edges(source, rel, target)';
    END IF;
  END IF;
END$$;

