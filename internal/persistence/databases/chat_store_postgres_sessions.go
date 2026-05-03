package databases

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"manifold/internal/observability"
	"manifold/internal/persistence"
)

func hasAccess(userID *int64, owner *int64) bool {
	if userID == nil {
		return true
	}
	if owner == nil {
		return false
	}
	return *userID == *owner
}

func (s *pgChatStore) scanSession(row pgx.Row) (persistence.ChatSession, error) {
	var cs persistence.ChatSession
	var owner sql.NullInt64
	if err := row.Scan(&cs.ID, &cs.Name, &owner, &cs.CreatedAt, &cs.UpdatedAt, &cs.LastMessagePreview, &cs.Model, &cs.Summary, &cs.SummarizedCount); err != nil {
		return persistence.ChatSession{}, err
	}
	if owner.Valid {
		v := owner.Int64
		cs.UserID = &v
	}
	return cs, nil
}

func (s *pgChatStore) lookupSessionOwner(ctx context.Context, id string) (*int64, error) {
	row := s.pool.QueryRow(ctx, `SELECT user_id FROM chat_sessions WHERE id = $1`, id)
	var owner sql.NullInt64
	if err := row.Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	if !owner.Valid {
		return nil, nil
	}
	v := owner.Int64
	return &v, nil
}

func (s *pgChatStore) EnsureSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(id) == "" {
		return persistence.ChatSession{}, errors.New("id required")
	}
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	var uid any
	if userID != nil {
		uid = *userID
	}
	row := s.pool.QueryRow(ctx, `
WITH ins AS (
  INSERT INTO chat_sessions (id, user_id, name)
  VALUES ($1, $2, $3)
  ON CONFLICT (id) DO NOTHING
  RETURNING id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count
)
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count FROM ins
UNION ALL
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count FROM chat_sessions WHERE id = $1
LIMIT 1`, id, uid, name)
	cs, err := s.scanSession(row)
	if err != nil {
		return persistence.ChatSession{}, err
	}
	if !hasAccess(userID, cs.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	return cs, nil
}

func (s *pgChatStore) ListSessions(ctx context.Context, userID *int64) ([]persistence.ChatSession, error) {
	query := `
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count
FROM chat_sessions`
	args := []any{}
	if userID != nil {
		query += `
WHERE user_id = $1`
		args = append(args, *userID)
	}
	query += `
ORDER BY updated_at DESC, created_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []persistence.ChatSession
	for rows.Next() {
		cs, err := s.scanSession(rows)
		if err != nil {
			return nil, err
		}
		if !hasAccess(userID, cs.UserID) {
			continue
		}
		out = append(out, cs)
	}
	if out == nil {
		out = make([]persistence.ChatSession, 0)
	}
	return out, rows.Err()
}

func (s *pgChatStore) GetSession(ctx context.Context, userID *int64, id string) (persistence.ChatSession, error) {
	log := observability.LoggerWithTrace(ctx)
	query := `
SELECT id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count
FROM chat_sessions
WHERE id = $1`
	args := []any{id}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
		log.Debug().Int64("user_id", *userID).Str("session_id", id).Msg("get_session_with_userid")
	} else {
		log.Debug().Str("session_id", id).Msg("get_session_no_userid")
	}
	row := s.pool.QueryRow(ctx, query, args...)
	cs, err := s.scanSession(row)
	if err == nil {
		log.Debug().Str("session_id", id).Msg("get_session_found")
		return cs, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		log.Error().Err(err).Str("session_id", id).Msg("get_session_error")
		return persistence.ChatSession{}, err
	}
	log.Warn().Str("session_id", id).Msg("get_session_no_rows")
	if userID == nil {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	owner, ownerErr := s.lookupSessionOwner(ctx, id)
	if ownerErr != nil {
		return persistence.ChatSession{}, ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	return persistence.ChatSession{}, persistence.ErrNotFound
}

func (s *pgChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	id := uuid.New()
	var uid any
	if userID != nil {
		uid = *userID
	}
	row := s.pool.QueryRow(ctx, `
INSERT INTO chat_sessions (id, user_id, name)
VALUES ($1, $2, $3)
RETURNING id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count`, id, uid, name)
	return s.scanSession(row)
}

func (s *pgChatStore) RenameSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		return persistence.ChatSession{}, errors.New("name required")
	}
	query := `
UPDATE chat_sessions
SET name = $2, updated_at = NOW()
WHERE id = $1`
	args := []any{id, name}
	if userID != nil {
		query += ` AND user_id = $3`
		args = append(args, *userID)
	}
	query += `
RETURNING id, name, user_id, created_at, updated_at, last_message_preview, model, summary, summarized_count`
	row := s.pool.QueryRow(ctx, query, args...)
	cs, err := s.scanSession(row)
	if err == nil {
		return cs, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return persistence.ChatSession{}, err
	}
	if userID == nil {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	owner, ownerErr := s.lookupSessionOwner(ctx, id)
	if ownerErr != nil {
		return persistence.ChatSession{}, ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	return persistence.ChatSession{}, persistence.ErrNotFound
}

func (s *pgChatStore) DeleteSession(ctx context.Context, userID *int64, id string) error {
	query := `DELETE FROM chat_sessions WHERE id = $1`
	args := []any{id}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
	}
	cmd, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() > 0 {
		return nil
	}
	if userID == nil {
		return persistence.ErrNotFound
	}
	owner, ownerErr := s.lookupSessionOwner(ctx, id)
	if ownerErr != nil {
		return ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ErrForbidden
	}
	return persistence.ErrNotFound
}
