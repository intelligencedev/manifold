package databases

import (
	"context"
	"sync"
	"time"

	"manifold/internal/persistence"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewUserPreferencesStore returns a Postgres-backed store if a pool is provided,
// otherwise an in-memory store.
func NewUserPreferencesStore(pool *pgxpool.Pool) persistence.UserPreferencesStore {
	if pool == nil {
		return &memUserPreferencesStore{m: map[int64]persistence.UserPreferences{}}
	}
	return &pgUserPreferencesStore{pool: pool}
}

// memUserPreferencesStore is an in-memory implementation for simple deployments.
type memUserPreferencesStore struct {
	mu sync.RWMutex
	m  map[int64]persistence.UserPreferences
}

func (s *memUserPreferencesStore) Init(ctx context.Context) error { return nil }

func (s *memUserPreferencesStore) Get(ctx context.Context, userID int64) (persistence.UserPreferences, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if prefs, ok := s.m[userID]; ok {
		return prefs, nil
	}
	// Return zero-value with user ID set
	return persistence.UserPreferences{UserID: userID}, nil
}

func (s *memUserPreferencesStore) SetActiveProject(ctx context.Context, userID int64, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[userID] = persistence.UserPreferences{
		UserID:          userID,
		ActiveProjectID: projectID,
		UpdatedAt:       time.Now(),
	}
	return nil
}

// pgUserPreferencesStore is a PostgreSQL-backed implementation for auth-enabled deployments.
type pgUserPreferencesStore struct {
	pool *pgxpool.Pool
}

func (s *pgUserPreferencesStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS user_preferences (
    user_id BIGINT PRIMARY KEY,
    active_project_id TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_preferences_active_project
    ON user_preferences(active_project_id)
    WHERE active_project_id IS NOT NULL;
`)
	return err
}

func (s *pgUserPreferencesStore) Get(ctx context.Context, userID int64) (persistence.UserPreferences, error) {
	var prefs persistence.UserPreferences
	var activeProjectID *string

	err := s.pool.QueryRow(ctx, `
		SELECT user_id, active_project_id, updated_at
		FROM user_preferences
		WHERE user_id = $1
	`, userID).Scan(&prefs.UserID, &activeProjectID, &prefs.UpdatedAt)

	if err != nil {
		// If not found, return zero-value with user ID set
		return persistence.UserPreferences{UserID: userID}, nil
	}
	if activeProjectID != nil {
		prefs.ActiveProjectID = *activeProjectID
	}
	return prefs, nil
}

func (s *pgUserPreferencesStore) SetActiveProject(ctx context.Context, userID int64, projectID string) error {
	var activeProjectID *string
	if projectID != "" {
		activeProjectID = &projectID
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_preferences (user_id, active_project_id, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			active_project_id = EXCLUDED.active_project_id,
			updated_at = EXCLUDED.updated_at
	`, userID, activeProjectID)
	return err
}
