CREATE TABLE IF NOT EXISTS specialist_groups (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    orchestrator JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS specialist_groups_user_name_idx
    ON specialist_groups(user_id, name);

CREATE TABLE IF NOT EXISTS specialist_group_memberships (
    user_id BIGINT NOT NULL DEFAULT 0,
    group_id INT NOT NULL REFERENCES specialist_groups(id) ON DELETE CASCADE,
    specialist_name TEXT NOT NULL,
    PRIMARY KEY (user_id, group_id, specialist_name)
);
