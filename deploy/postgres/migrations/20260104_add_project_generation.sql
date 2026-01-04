-- Adds generation tracking columns to projects table
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS generation BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS skills_generation BIGINT NOT NULL DEFAULT 0;

-- Indexes to speed up staleness checks
CREATE INDEX IF NOT EXISTS idx_projects_generation ON projects (generation);
CREATE INDEX IF NOT EXISTS idx_projects_skills_generation ON projects (skills_generation);
