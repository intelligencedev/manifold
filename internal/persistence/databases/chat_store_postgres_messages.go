package databases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"manifold/internal/observability"
	"manifold/internal/persistence"
)

func (s *pgChatStore) DeleteMessageWithRelated(ctx context.Context, userID *int64, sessionID string, messageID string, relatedMessageIDs []string, resetSummary bool) error {
	if strings.TrimSpace(messageID) == "" {
		return persistence.ErrNotFound
	}
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cmd, err := tx.Exec(ctx, `DELETE FROM chat_messages WHERE session_id = $1 AND id = $2`, sessionID, messageID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrNotFound
	}
	if err := s.deleteRelatedMessagesTx(ctx, tx, sessionID, relatedMessageIDs); err != nil {
		return err
	}
	if err := s.finalizeChatDeleteTx(ctx, tx, userID, sessionID, resetSummary); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *pgChatStore) DeleteMessage(ctx context.Context, userID *int64, sessionID string, messageID string) error {
	if strings.TrimSpace(messageID) == "" {
		return persistence.ErrNotFound
	}
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cmd, err := tx.Exec(ctx, `DELETE FROM chat_messages WHERE session_id = $1 AND id = $2`, sessionID, messageID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrNotFound
	}
	if err := s.updatePreviewAfterDeleteTx(ctx, tx, userID, sessionID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *pgChatStore) DeleteMessagesAfterWithRelated(ctx context.Context, req persistence.ChatDeleteAfterRequest) error {
	if strings.TrimSpace(req.MessageID) == "" {
		return persistence.ErrNotFound
	}
	if _, err := s.GetSession(ctx, req.UserID, req.SessionID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.deleteMessagesAfterTx(ctx, tx, req.SessionID, req.MessageID, req.Inclusive); err != nil {
		return err
	}
	if err := s.deleteRelatedMessagesTx(ctx, tx, req.SessionID, req.RelatedMessageIDs); err != nil {
		return err
	}
	if err := s.finalizeChatDeleteTx(ctx, tx, req.UserID, req.SessionID, req.ResetSummary); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *pgChatStore) DeleteMessagesAfter(ctx context.Context, userID *int64, sessionID string, messageID string, inclusive bool) error {
	if strings.TrimSpace(messageID) == "" {
		return persistence.ErrNotFound
	}
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.deleteMessagesAfterTx(ctx, tx, sessionID, messageID, inclusive); err != nil {
		return err
	}
	if err := s.updatePreviewAfterDeleteTx(ctx, tx, userID, sessionID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *pgChatStore) ListMessages(ctx context.Context, userID *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	log := observability.LoggerWithTrace(ctx)
	log.Debug().Str("session_id", sessionID).Int("limit", limit).Msg("list_messages_start")
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("list_messages_get_session_failed")
		return nil, err
	}
	log.Debug().Str("session_id", sessionID).Msg("list_messages_session_ok")
	query := `
SELECT id, session_id, role, content, created_at
FROM chat_messages
WHERE session_id = $1
ORDER BY created_at ASC, id ASC`
	args := []any{sessionID}
	if limit > 0 {
		query = `
SELECT id, session_id, role, content, created_at FROM (
    SELECT id, session_id, role, content, created_at
    FROM chat_messages
    WHERE session_id = $1
    ORDER BY created_at DESC, id DESC
    LIMIT $2
) sub
ORDER BY created_at ASC, id ASC`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persistence.ChatMessage
	for rows.Next() {
		var msg persistence.ChatMessage
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if out == nil {
		out = make([]persistence.ChatMessage, 0)
	}
	log.Debug().Str("session_id", sessionID).Int("message_count", len(out)).Msg("list_messages_complete")
	return out, rows.Err()
}

func (s *pgChatStore) AppendMessages(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	return s.appendMessages(chatAppendMessagesRequest{ctx: ctx, userID: userID, sessionID: sessionID, messages: messages, preview: preview, model: model})
}

func (s *pgChatStore) AppendMessagesOnce(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	return s.appendMessages(chatAppendMessagesRequest{ctx: ctx, userID: userID, sessionID: sessionID, messages: messages, preview: preview, model: model, skipExisting: true})
}

func (s *pgChatStore) appendMessages(req chatAppendMessagesRequest) error {
	if len(req.messages) == 0 {
		return nil
	}
	if _, err := s.GetSession(req.ctx, req.userID, req.sessionID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(req.ctx, 5*time.Second)
	defer cancel()

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, message := range req.messages {
		id := message.ID
		if id == "" {
			id = uuid.NewString()
		}
		createdAt := message.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		query := `
	INSERT INTO chat_messages (id, session_id, role, content, created_at)
	VALUES ($1, $2, $3, $4, $5)`
		if req.skipExisting {
			query += `
	ON CONFLICT (id) DO NOTHING`
		}
		if _, err := tx.Exec(ctx, query, id, req.sessionID, message.Role, message.Content, createdAt); err != nil {
			return err
		}
	}

	modelUpdate := strings.TrimSpace(req.model)
	query := `
UPDATE chat_sessions
SET updated_at = NOW(),
    last_message_preview = $2,
    model = CASE WHEN $3 = '' THEN model ELSE $3 END
WHERE id = $1`
	args := []any{req.sessionID, req.preview, modelUpdate}
	if req.userID != nil {
		query += ` AND user_id = $4`
		args = append(args, *req.userID)
	}
	cmd, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return persistence.ErrForbidden
	}

	return tx.Commit(ctx)
}

func (s *pgChatStore) deleteMessagesAfterTx(ctx context.Context, tx pgx.Tx, sessionID string, messageID string, inclusive bool) error {
	var targetCreated time.Time
	row := tx.QueryRow(ctx, `SELECT created_at FROM chat_messages WHERE session_id = $1 AND id = $2`, sessionID, messageID)
	if err := row.Scan(&targetCreated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persistence.ErrNotFound
		}
		return err
	}

	cmp := ">"
	if inclusive {
		cmp = ">="
	}
	query := `
DELETE FROM chat_messages
WHERE session_id = $1
AND (created_at > $2 OR (created_at = $2 AND id ` + cmp + ` $3))`
	_, err := tx.Exec(ctx, query, sessionID, targetCreated, messageID)
	return err
}

func (s *pgChatStore) deleteRelatedMessagesTx(ctx context.Context, tx pgx.Tx, sessionID string, relatedMessageIDs []string) error {
	if len(relatedMessageIDs) == 0 {
		return nil
	}
	for _, relatedID := range relatedMessageIDs {
		relatedID = strings.TrimSpace(relatedID)
		if relatedID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `DELETE FROM chat_messages WHERE session_id = $1 AND id = $2`, sessionID, relatedID); err != nil {
			return err
		}
	}
	return nil
}

func (s *pgChatStore) finalizeChatDeleteTx(ctx context.Context, tx pgx.Tx, userID *int64, sessionID string, resetSummary bool) error {
	if resetSummary {
		query := `
UPDATE chat_sessions
SET summary = '', summarized_count = 0, updated_at = NOW()
WHERE id = $1`
		args := []any{sessionID}
		if userID != nil {
			query += ` AND user_id = $2`
			args = append(args, *userID)
		}
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
	}

	return s.updatePreviewAfterDeleteTx(ctx, tx, userID, sessionID)
}

func (s *pgChatStore) updatePreviewAfterDeleteTx(ctx context.Context, tx pgx.Tx, userID *int64, sessionID string) error {
	var lastContent string
	row := tx.QueryRow(ctx, `
SELECT content
FROM chat_messages
WHERE session_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1`, sessionID)
	if err := row.Scan(&lastContent); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		lastContent = ""
	}

	preview := snippetForPreview(lastContent)
	update := `UPDATE chat_sessions SET updated_at = NOW(), last_message_preview = $2 WHERE id = $1`
	args := []any{sessionID, preview}
	if userID != nil {
		update += ` AND user_id = $3`
		args = append(args, *userID)
	}
	_, err := tx.Exec(ctx, update, args...)
	return err
}

func (s *pgChatStore) UpdateSummary(ctx context.Context, userID *int64, sessionID string, summary string, summarizedCount int) error {
	query := `
UPDATE chat_sessions
SET summary = $2, summarized_count = $3, updated_at = NOW()
WHERE id = $1`
	args := []any{sessionID, summary, summarizedCount}
	if userID != nil {
		query += ` AND user_id = $4`
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
	owner, ownerErr := s.lookupSessionOwner(ctx, sessionID)
	if ownerErr != nil {
		return ownerErr
	}
	if !hasAccess(userID, owner) {
		return persistence.ErrForbidden
	}
	return persistence.ErrNotFound
}
