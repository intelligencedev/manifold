package databases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/persistence"
)

func NewPostgresMatrixMessageStore(pool *pgxpool.Pool) persistence.MatrixMessageStore {
	if pool == nil {
		return newMemoryMatrixMessageStore()
	}
	return &pgMatrixMessageStore{pool: pool}
}

type pgMatrixMessageStore struct {
	pool *pgxpool.Pool
}

func (s *pgMatrixMessageStore) Init(ctx context.Context) error {
	if s.pool == nil {
		return errors.New("postgres matrix message store requires pool")
	}
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS matrix_messages (
    id BIGSERIAL PRIMARY KEY,
    room_id TEXT NOT NULL,
    event_id TEXT,
    direction TEXT NOT NULL,
    sender TEXT NOT NULL DEFAULT '',
    target TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    formatted_body TEXT NOT NULL DEFAULT '',
    msg_type TEXT NOT NULL DEFAULT 'm.text',
    media_url TEXT NOT NULL DEFAULT '',
    media_mime TEXT NOT NULL DEFAULT '',
    media_size BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS matrix_messages_event_idx
    ON matrix_messages(event_id)
    WHERE event_id IS NOT NULL AND event_id <> '';

CREATE INDEX IF NOT EXISTS matrix_messages_room_created_idx
    ON matrix_messages(room_id, created_at DESC, id DESC);
`)
	return err
}

func (s *pgMatrixMessageStore) Append(ctx context.Context, message persistence.MatrixMessage, maxMessages int) (persistence.MatrixMessage, error) {
	message.RoomID = strings.TrimSpace(message.RoomID)
	message.EventID = strings.TrimSpace(message.EventID)
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return persistence.MatrixMessage{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
INSERT INTO matrix_messages (room_id, event_id, direction, sender, target, body, formatted_body, msg_type, media_url, media_mime, media_size, created_at)
VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (event_id) DO UPDATE SET event_id = EXCLUDED.event_id
RETURNING id, room_id, COALESCE(event_id, ''), direction, sender, target, body, formatted_body, msg_type, media_url, media_mime, media_size, created_at`,
		message.RoomID, message.EventID, message.Direction, message.Sender, message.Target, message.Body, message.FormattedBody, message.MsgType, message.MediaURL, message.MediaMIME, message.MediaSize, message.CreatedAt)
	if err := scanMatrixMessage(row, &message); err != nil {
		return persistence.MatrixMessage{}, err
	}
	if maxMessages > 0 {
		if err := pruneMatrixMessagesTx(ctx, tx, message.RoomID, maxMessages); err != nil {
			return persistence.MatrixMessage{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return persistence.MatrixMessage{}, err
	}
	return message, nil
}

func (s *pgMatrixMessageStore) ListByRoom(ctx context.Context, roomID string, limit int, beforeID int64) ([]persistence.MatrixMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, room_id, COALESCE(event_id, ''), direction, sender, target, body, formatted_body, msg_type, media_url, media_mime, media_size, created_at
FROM matrix_messages
WHERE room_id = $1 AND ($2::BIGINT = 0 OR id < $2)
ORDER BY created_at DESC, id DESC
LIMIT $3`, strings.TrimSpace(roomID), beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]persistence.MatrixMessage, 0)
	for rows.Next() {
		var msg persistence.MatrixMessage
		if err := scanMatrixMessage(rows, &msg); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if out == nil {
		out = []persistence.MatrixMessage{}
	}
	return out, rows.Err()
}

func (s *pgMatrixMessageStore) Prune(ctx context.Context, roomID string, maxMessages int) error {
	if maxMessages <= 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
DELETE FROM matrix_messages
WHERE room_id = $1
  AND id NOT IN (
    SELECT id FROM matrix_messages
    WHERE room_id = $1
    ORDER BY created_at DESC, id DESC
    LIMIT $2
  )`, strings.TrimSpace(roomID), maxMessages)
	return err
}

func (s *pgMatrixMessageStore) RoomStats(ctx context.Context, roomID string) (persistence.MatrixRoomStats, error) {
	stats := persistence.MatrixRoomStats{RoomID: strings.TrimSpace(roomID)}
	row := s.pool.QueryRow(ctx, `
SELECT COUNT(*), COALESCE(MAX(created_at), TIMESTAMPTZ 'epoch')
FROM matrix_messages WHERE room_id = $1`, stats.RoomID)
	if err := row.Scan(&stats.MessageCount, &stats.LastActivityAt); err != nil {
		return persistence.MatrixRoomStats{}, err
	}
	lastSenderRow := s.pool.QueryRow(ctx, `
SELECT sender
FROM matrix_messages
WHERE room_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1`, stats.RoomID)
	_ = lastSenderRow.Scan(&stats.LastSender)
	if stats.LastActivityAt.Equal(time.Unix(0, 0).UTC()) {
		stats.LastActivityAt = time.Time{}
	}
	return stats, nil
}

func (s *pgMatrixMessageStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func scanMatrixMessage(row interface{ Scan(dest ...any) error }, msg *persistence.MatrixMessage) error {
	return row.Scan(&msg.ID, &msg.RoomID, &msg.EventID, &msg.Direction, &msg.Sender, &msg.Target, &msg.Body, &msg.FormattedBody, &msg.MsgType, &msg.MediaURL, &msg.MediaMIME, &msg.MediaSize, &msg.CreatedAt)
}

func pruneMatrixMessagesTx(ctx context.Context, tx pgx.Tx, roomID string, maxMessages int) error {
	_, err := tx.Exec(ctx, `
DELETE FROM matrix_messages
WHERE room_id = $1
  AND id NOT IN (
    SELECT id FROM matrix_messages
    WHERE room_id = $1
    ORDER BY created_at DESC, id DESC
    LIMIT $2
  )`, roomID, maxMessages)
	return err
}
