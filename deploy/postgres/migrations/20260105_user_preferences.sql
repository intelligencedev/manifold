-- User Preferences table for persisting user settings across sessions.
-- Primary use case: storing active project selection for auth-enabled workspace mode.

CREATE TABLE IF NOT EXISTS user_preferences (
    user_id BIGINT PRIMARY KEY,
    active_project_id TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for efficient active_project_id lookups (useful for cleanup queries)
CREATE INDEX IF NOT EXISTS idx_user_preferences_active_project
    ON user_preferences(active_project_id)
    WHERE active_project_id IS NOT NULL;

-- Comment documenting the table's purpose
COMMENT ON TABLE user_preferences IS 'Stores user-specific preferences, notably active project selection for workspace management';
COMMENT ON COLUMN user_preferences.user_id IS 'References the authenticated user (0 for anonymous/system)';
COMMENT ON COLUMN user_preferences.active_project_id IS 'Currently selected project ID for workspace binding';
COMMENT ON COLUMN user_preferences.updated_at IS 'Last modification timestamp';
