package databases

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/persistence"
)

func NewSQLiteChatStore(db *sql.DB) persistence.ChatStore {
	return &sqliteChatStore{db: db}
}

type sqliteChatStore struct {
	db *sql.DB
}

type sqliteChatAppendRequest struct {
	UserID       *int64
	SessionID    string
	Messages     []persistence.ChatMessage
	Preview      string
	Model        string
	SkipExisting bool
}

func (s *sqliteChatStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite chat store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS chat_sessions (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'chat',
	user_id INTEGER,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_message_preview TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	summary TEXT NOT NULL DEFAULT '',
	summarized_count INTEGER NOT NULL DEFAULT 0,
	project_id TEXT NOT NULL DEFAULT '',
	memory_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	evolving_memory_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	belief_memory_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	active_specialist TEXT NOT NULL DEFAULT '',
	active_team TEXT NOT NULL DEFAULT '',
	pinned BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS chat_messages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	duration_ms INTEGER NULL
);
CREATE INDEX IF NOT EXISTS chat_messages_session_created_idx ON chat_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS chat_sessions_user_updated_idx ON chat_sessions(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS chat_sessions_user_created_idx ON chat_sessions(user_id, created_at DESC);
`)
	if err != nil {
		return err
	}
	if err := ensureSQLiteColumn(ctx, s.db, "chat_messages", "duration_ms", "INTEGER NULL"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(ctx, s.db, "chat_sessions", "active_specialist", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureSQLiteColumn(ctx, s.db, "chat_sessions", "active_team", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return ensureSQLiteColumn(ctx, s.db, "chat_sessions", "pinned", "BOOLEAN NOT NULL DEFAULT FALSE")
}

func (s *sqliteChatStore) EnsureSession(ctx context.Context, userID *int64, id string, name string) (persistence.ChatSession, error) {
	return s.EnsureSessionKind(ctx, userID, id, name, persistence.ChatSessionKindChat)
}

func (s *sqliteChatStore) EnsureSessionKind(ctx context.Context, userID *int64, id string, name string, kind string) (persistence.ChatSession, error) {
	if strings.TrimSpace(id) == "" {
		return persistence.ChatSession{}, errors.New("id required")
	}
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	kind = normalizeSessionKind(kind)
	if err := s.Init(ctx); err != nil {
		return persistence.ChatSession{}, err
	}
	var uid any
	if userID != nil {
		uid = *userID
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO chat_sessions(id, user_id, name, kind) VALUES(?, ?, ?, ?)`, id, uid, name, kind)
	if err != nil {
		return persistence.ChatSession{}, err
	}
	return s.GetSession(ctx, userID, id)
}

func (s *sqliteChatStore) ListSessions(ctx context.Context, userID *int64) ([]persistence.ChatSession, error) {
	return s.ListSessionsByKind(ctx, userID, persistence.ChatSessionKindChat)
}

func (s *sqliteChatStore) ListSessionsByKind(ctx context.Context, userID *int64, kind string) ([]persistence.ChatSession, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	kind = normalizeSessionKind(kind)
	query := `SELECT id, name, kind, user_id, created_at, updated_at, last_message_preview,
		(SELECT COUNT(*) FROM chat_messages m WHERE m.session_id = chat_sessions.id) AS message_count,
		model, summary, summarized_count, project_id, memory_enabled, evolving_memory_enabled, belief_memory_enabled, active_specialist, active_team, pinned
		FROM chat_sessions WHERE kind = ?`
	args := []any{kind}
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	query += ` ORDER BY pinned DESC, updated_at DESC, created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persistence.ChatSession{}
	for rows.Next() {
		session, err := scanSQLiteChatSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (s *sqliteChatStore) GetSession(ctx context.Context, userID *int64, id string) (persistence.ChatSession, error) {
	if err := s.Init(ctx); err != nil {
		return persistence.ChatSession{}, err
	}
	query := `SELECT id, name, kind, user_id, created_at, updated_at, last_message_preview,
		(SELECT COUNT(*) FROM chat_messages m WHERE m.session_id = chat_sessions.id) AS message_count,
		model, summary, summarized_count, project_id, memory_enabled, evolving_memory_enabled, belief_memory_enabled, active_specialist, active_team, pinned
		FROM chat_sessions WHERE id = ?`
	args := []any{id}
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	row := s.db.QueryRowContext(ctx, query, args...)
	session, err := scanSQLiteChatSession(row)
	if err == nil {
		return session, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
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

func (s *sqliteChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	return s.CreateSessionKind(ctx, userID, name, persistence.ChatSessionKindChat)
}

func (s *sqliteChatStore) CreateSessionKind(ctx context.Context, userID *int64, name string, kind string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	return s.EnsureSessionKind(ctx, userID, uuid.NewString(), name, kind)
}

func (s *sqliteChatStore) RenameSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		return persistence.ChatSession{}, errors.New("name required")
	}
	return s.updateSessionReturning(ctx, userID, id, `name = ?, updated_at = CURRENT_TIMESTAMP`, name)
}

func (s *sqliteChatStore) SetSessionProject(ctx context.Context, userID *int64, id, projectID string) (persistence.ChatSession, error) {
	return s.updateSessionReturning(ctx, userID, id, `project_id = ?, updated_at = CURRENT_TIMESTAMP`, strings.TrimSpace(projectID))
}

func (s *sqliteChatStore) SetSessionMemorySettings(ctx context.Context, userID *int64, id string, memoryEnabled bool, evolvingMemoryEnabled bool, beliefMemoryEnabled bool) (persistence.ChatSession, error) {
	return s.updateSessionReturning(ctx, userID, id, `memory_enabled = ?, evolving_memory_enabled = ?, belief_memory_enabled = ?, updated_at = CURRENT_TIMESTAMP`, memoryEnabled, evolvingMemoryEnabled, beliefMemoryEnabled)
}

func (s *sqliteChatStore) SetSessionActiveTarget(ctx context.Context, userID *int64, id string, activeSpecialist string, activeTeam string) (persistence.ChatSession, error) {
	return s.updateSessionReturning(ctx, userID, id, `active_specialist = ?, active_team = ?`, strings.TrimSpace(activeSpecialist), strings.TrimSpace(activeTeam))
}

func (s *sqliteChatStore) SetSessionPinned(ctx context.Context, userID *int64, id string, pinned bool) (persistence.ChatSession, error) {
	return s.updateSessionReturning(ctx, userID, id, `pinned = ?`, pinned)
}

func (s *sqliteChatStore) DeleteSession(ctx context.Context, userID *int64, id string) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	query := `DELETE FROM chat_sessions WHERE id = ?`
	args := []any{id}
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
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

func (s *sqliteChatStore) ListMessages(ctx context.Context, userID *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		return nil, err
	}
	query := `SELECT id, session_id, role, content, created_at, duration_ms FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC, id ASC`
	args := []any{sessionID}
	if limit > 0 {
		query = `SELECT id, session_id, role, content, created_at, duration_ms FROM (
			SELECT id, session_id, role, content, created_at, duration_ms
			FROM chat_messages
			WHERE session_id = ?
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		) ORDER BY created_at ASC, id ASC`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persistence.ChatMessage{}
	for rows.Next() {
		var msg persistence.ChatMessage
		var createdAt sqliteTime
		var duration sql.NullInt64
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &createdAt, &duration); err != nil {
			return nil, err
		}
		msg.CreatedAt = createdAt.Time
		msg.DurationMs = int64PtrFromNull(duration)
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *sqliteChatStore) ListMessagesBefore(ctx context.Context, userID *int64, sessionID string, beforeID string, limit int) ([]persistence.ChatMessage, error) {
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT id, session_id, role, content, created_at, duration_ms FROM (
		SELECT m.id, m.session_id, m.role, m.content, m.created_at, m.duration_ms
		FROM chat_messages m
		JOIN chat_messages cursor ON cursor.session_id = ? AND cursor.id = ?
		WHERE m.session_id = ?
			AND (m.created_at < cursor.created_at OR (m.created_at = cursor.created_at AND m.id < cursor.id))
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT ?
	) ORDER BY created_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, query, sessionID, beforeID, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persistence.ChatMessage{}
	for rows.Next() {
		var msg persistence.ChatMessage
		var createdAt sqliteTime
		var duration sql.NullInt64
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &createdAt, &duration); err != nil {
			return nil, err
		}
		msg.CreatedAt = createdAt.Time
		msg.DurationMs = int64PtrFromNull(duration)
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (s *sqliteChatStore) DeleteMessage(ctx context.Context, userID *int64, sessionID string, messageID string) error {
	return s.DeleteMessageWithRelated(ctx, userID, sessionID, messageID, nil, false)
}

func (s *sqliteChatStore) DeleteMessagesAfter(ctx context.Context, userID *int64, sessionID string, messageID string, inclusive bool) error {
	return s.DeleteMessagesAfterWithRelated(ctx, persistence.ChatDeleteAfterRequest{UserID: userID, SessionID: sessionID, MessageID: messageID, Inclusive: inclusive})
}

func (s *sqliteChatStore) AppendMessages(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	return s.appendMessages(ctx, sqliteChatAppendRequest{UserID: userID, SessionID: sessionID, Messages: messages, Preview: preview, Model: model})
}

func (s *sqliteChatStore) AppendMessagesOnce(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	return s.appendMessages(ctx, sqliteChatAppendRequest{UserID: userID, SessionID: sessionID, Messages: messages, Preview: preview, Model: model, SkipExisting: true})
}

func (s *sqliteChatStore) UpdateSummary(ctx context.Context, userID *int64, sessionID string, summary string, summarizedCount int) error {
	if err := s.updateSession(ctx, userID, sessionID, `summary = ?, summarized_count = ?, updated_at = CURRENT_TIMESTAMP`, summary, summarizedCount); err != nil {
		return err
	}
	return nil
}

func (s *sqliteChatStore) DeleteMessageWithRelated(ctx context.Context, userID *int64, sessionID string, messageID string, relatedMessageIDs []string, resetSummary bool) error {
	if strings.TrimSpace(messageID) == "" {
		return persistence.ErrNotFound
	}
	if _, err := s.GetSession(ctx, userID, sessionID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	res, err := tx.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ? AND id = ?`, sessionID, messageID)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return persistence.ErrNotFound
	}
	if err := sqliteDeleteRelatedMessages(ctx, tx, sessionID, relatedMessageIDs); err != nil {
		return err
	}
	if err := s.finalizeSQLiteChatDeleteTx(ctx, tx, userID, sessionID, resetSummary); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteChatStore) DeleteMessagesAfterWithRelated(ctx context.Context, req persistence.ChatDeleteAfterRequest) error {
	if strings.TrimSpace(req.MessageID) == "" {
		return persistence.ErrNotFound
	}
	if _, err := s.GetSession(ctx, req.UserID, req.SessionID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if err := sqliteDeleteMessagesAfter(ctx, tx, req.SessionID, req.MessageID, req.Inclusive); err != nil {
		return err
	}
	if err := sqliteDeleteRelatedMessages(ctx, tx, req.SessionID, req.RelatedMessageIDs); err != nil {
		return err
	}
	if err := s.finalizeSQLiteChatDeleteTx(ctx, tx, req.UserID, req.SessionID, req.ResetSummary); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteChatStore) appendMessages(ctx context.Context, req sqliteChatAppendRequest) error {
	if len(req.Messages) == 0 {
		return nil
	}
	if _, err := s.GetSession(ctx, req.UserID, req.SessionID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	for _, message := range req.Messages {
		id := strings.TrimSpace(message.ID)
		if id == "" {
			id = uuid.NewString()
		}
		createdAt := message.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		stmt := `INSERT INTO chat_messages(id, session_id, role, content, created_at, duration_ms) VALUES(?, ?, ?, ?, ?, ?)`
		if req.SkipExisting {
			stmt = `INSERT OR IGNORE INTO chat_messages(id, session_id, role, content, created_at, duration_ms) VALUES(?, ?, ?, ?, ?, ?)`
		}
		if _, err := tx.ExecContext(ctx, stmt, id, req.SessionID, message.Role, message.Content, createdAt, nullableInt64Value(message.DurationMs)); err != nil {
			return err
		}
	}
	model := strings.TrimSpace(req.Model)
	if _, err := tx.ExecContext(ctx, `
UPDATE chat_sessions
SET updated_at = CURRENT_TIMESTAMP,
	last_message_preview = ?,
	model = CASE WHEN ? = '' THEN model ELSE ? END
WHERE id = ?`, req.Preview, model, model, req.SessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureSQLiteColumn(ctx context.Context, db *sql.DB, table, column, definition string) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue any
			pk           int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, definition))
	return err
}

func (s *sqliteChatStore) updateSessionReturning(ctx context.Context, userID *int64, id, assignments string, args ...any) (persistence.ChatSession, error) {
	if err := s.updateSession(ctx, userID, id, assignments, args...); err != nil {
		return persistence.ChatSession{}, err
	}
	return s.GetSession(ctx, userID, id)
}

func (s *sqliteChatStore) updateSession(ctx context.Context, userID *int64, id, assignments string, args ...any) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	query := `UPDATE chat_sessions SET ` + assignments + ` WHERE id = ?`
	args = append(args, id)
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
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

func (s *sqliteChatStore) lookupSessionOwner(ctx context.Context, id string) (*int64, error) {
	var owner sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM chat_sessions WHERE id = ?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, persistence.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !owner.Valid {
		return nil, nil
	}
	v := owner.Int64
	return &v, nil
}

func (s *sqliteChatStore) finalizeSQLiteChatDeleteTx(ctx context.Context, tx *sql.Tx, userID *int64, sessionID string, resetSummary bool) error {
	if resetSummary {
		if err := sqliteUpdateSessionTx(ctx, tx, userID, sessionID, `summary = '', summarized_count = 0, updated_at = CURRENT_TIMESTAMP`); err != nil {
			return err
		}
	}
	var lastContent string
	err := tx.QueryRowContext(ctx, `SELECT content FROM chat_messages WHERE session_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, sessionID).Scan(&lastContent)
	if errors.Is(err, sql.ErrNoRows) {
		lastContent = ""
	} else if err != nil {
		return err
	}
	return sqliteUpdateSessionTx(ctx, tx, userID, sessionID, `updated_at = CURRENT_TIMESTAMP, last_message_preview = ?`, snippetForPreview(lastContent))
}

func sqliteDeleteMessagesAfter(ctx context.Context, tx *sql.Tx, sessionID string, messageID string, inclusive bool) error {
	var targetCreated string
	err := tx.QueryRowContext(ctx, `SELECT created_at FROM chat_messages WHERE session_id = ? AND id = ?`, sessionID, messageID).Scan(&targetCreated)
	if errors.Is(err, sql.ErrNoRows) {
		return persistence.ErrNotFound
	}
	if err != nil {
		return err
	}
	cmp := ">"
	if inclusive {
		cmp = ">="
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ? AND (created_at > ? OR (created_at = ? AND id `+cmp+` ?))`, sessionID, targetCreated, targetCreated, messageID)
	return err
}

func sqliteDeleteRelatedMessages(ctx context.Context, tx *sql.Tx, sessionID string, relatedMessageIDs []string) error {
	for _, relatedID := range relatedMessageIDs {
		relatedID = strings.TrimSpace(relatedID)
		if relatedID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ? AND id = ?`, sessionID, relatedID); err != nil {
			return err
		}
	}
	return nil
}

func sqliteUpdateSessionTx(ctx context.Context, tx *sql.Tx, userID *int64, sessionID string, assignments string, args ...any) error {
	query := `UPDATE chat_sessions SET ` + assignments + ` WHERE id = ?`
	args = append(args, sessionID)
	if userID != nil {
		query += ` AND user_id = ?`
		args = append(args, *userID)
	}
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func scanSQLiteChatSession(row interface {
	Scan(dest ...any) error
}) (persistence.ChatSession, error) {
	var session persistence.ChatSession
	var owner sql.NullInt64
	var createdAt sqliteTime
	var updatedAt sqliteTime
	if err := row.Scan(&session.ID, &session.Name, &session.Kind, &owner, &createdAt, &updatedAt, &session.LastMessagePreview, &session.MessageCount, &session.Model, &session.Summary, &session.SummarizedCount, &session.ProjectID, &session.MemoryEnabled, &session.EvolvingMemoryEnabled, &session.BeliefMemoryEnabled, &session.ActiveSpecialist, &session.ActiveTeam, &session.Pinned); err != nil {
		return persistence.ChatSession{}, err
	}
	session.CreatedAt = createdAt.Time
	session.UpdatedAt = updatedAt.Time
	if session.Kind == "" {
		session.Kind = persistence.ChatSessionKindChat
	}
	if owner.Valid {
		v := owner.Int64
		session.UserID = &v
	}
	return session, nil
}
