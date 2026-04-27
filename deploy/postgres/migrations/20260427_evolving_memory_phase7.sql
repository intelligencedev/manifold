ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'session';

ALTER TABLE evolving_memories
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS evolving_memories_user_scope_created_idx
    ON evolving_memories(user_id, scope, created_at DESC);

CREATE INDEX IF NOT EXISTS evolving_memories_expires_at_idx
    ON evolving_memories(expires_at)
    WHERE expires_at IS NOT NULL;
