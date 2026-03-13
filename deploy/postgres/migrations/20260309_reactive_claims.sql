CREATE TABLE IF NOT EXISTS reactive_claims (
    room_id TEXT PRIMARY KEY,
    bot_id TEXT NOT NULL,
    claim_token TEXT NOT NULL,
    trigger_event_id TEXT,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_reactive_claims_expires_at ON reactive_claims(expires_at);