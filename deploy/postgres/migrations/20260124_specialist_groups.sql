CREATE TABLE IF NOT EXISTS specialist_teams (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    orchestrator JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS specialist_teams_user_name_idx
    ON specialist_teams(user_id, name);

CREATE TABLE IF NOT EXISTS specialist_team_memberships (
    user_id BIGINT NOT NULL DEFAULT 0,
    team_id INT NOT NULL REFERENCES specialist_teams(id) ON DELETE CASCADE,
    specialist_name TEXT NOT NULL,
    PRIMARY KEY (user_id, team_id, specialist_name)
);
