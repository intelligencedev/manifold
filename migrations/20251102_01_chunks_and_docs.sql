-- Milestone 2: chunk-centric table and document enhancements (idempotent)
-- Safe to run multiple times; uses IF NOT EXISTS and avoids destructive changes.

-- documents: add columns if not present
ALTER TABLE IF EXISTS documents ADD COLUMN IF NOT EXISTS title   TEXT;
ALTER TABLE IF EXISTS documents ADD COLUMN IF NOT EXISTS url     TEXT;
ALTER TABLE IF EXISTS documents ADD COLUMN IF NOT EXISTS lang    regconfig DEFAULT 'english';
ALTER TABLE IF EXISTS documents ADD COLUMN IF NOT EXISTS doc_hash TEXT;
ALTER TABLE IF EXISTS documents ADD COLUMN IF NOT EXISTS version INT DEFAULT 1;
ALTER TABLE IF EXISTS documents ADD COLUMN IF NOT EXISTS source  TEXT;
ALTER TABLE IF EXISTS documents ADD COLUMN IF NOT EXISTS acl     JSONB DEFAULT '{}'::jsonb;

-- documents: helpful indexes
DO $$
BEGIN
  IF to_regclass('public.documents_doc_hash_uniq') IS NULL THEN
    EXECUTE 'CREATE UNIQUE INDEX documents_doc_hash_uniq ON documents(doc_hash) WHERE doc_hash IS NOT NULL';
  END IF;
  IF to_regclass('public.documents_metadata_idx') IS NULL THEN
    EXECUTE 'CREATE INDEX documents_metadata_idx ON documents USING GIN (metadata)';
  END IF;
  IF to_regclass('public.documents_version_idx') IS NULL THEN
    EXECUTE 'CREATE INDEX documents_version_idx ON documents (version)';
  END IF;
END$$;

-- chunks table
CREATE TABLE IF NOT EXISTS chunks (
  id        TEXT PRIMARY KEY,
  doc_id    TEXT NOT NULL,
  idx       INT  NOT NULL,
  text      TEXT NOT NULL,
  metadata  JSONB NOT NULL DEFAULT '{}'::jsonb,
  lang      regconfig DEFAULT 'english',
  ts        tsvector GENERATED ALWAYS AS (to_tsvector(lang, coalesce(text,''))) STORED,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- chunks indexes (GIN for ts and metadata; BTREE for doc_id,idx)
DO $$
BEGIN
  IF to_regclass('public.chunks_ts_idx') IS NULL THEN
    EXECUTE 'CREATE INDEX chunks_ts_idx ON chunks USING GIN (ts)';
  END IF;
  IF to_regclass('public.chunks_metadata_idx') IS NULL THEN
    EXECUTE 'CREATE INDEX chunks_metadata_idx ON chunks USING GIN (metadata)';
  END IF;
  IF to_regclass('public.chunks_doc_idx') IS NULL THEN
    EXECUTE 'CREATE INDEX chunks_doc_idx ON chunks (doc_id, idx)';
  END IF;
END$$;

