-- Add lexical retrieval support for evolving memory hybrid search.

ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS search_tsv tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(input, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(strategy_card, ''))
    ) STORED;

CREATE INDEX IF NOT EXISTS evolving_memories_search_tsv_idx
    ON evolving_memories USING GIN (search_tsv);
