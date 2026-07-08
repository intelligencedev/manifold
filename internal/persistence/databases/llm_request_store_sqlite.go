package databases

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"manifold/internal/persistence"
)

func NewSQLiteLLMRequestStore(db *sql.DB, chat persistence.ChatStore) persistence.LLMRequestStore {
	return &sqliteLLMRequestStore{db: db, chat: chat}
}

type sqliteLLMRequestStore struct {
	db   *sql.DB
	chat persistence.ChatStore
}

func (s *sqliteLLMRequestStore) Init(ctx context.Context) error {
	if s.db == nil {
		return errors.New("sqlite llm request store requires db")
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS llm_requests (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
	user_id INTEGER,
	run_id TEXT NOT NULL DEFAULT '',
	message_id TEXT NOT NULL DEFAULT '',
	parent_user_message_id TEXT NOT NULL DEFAULT '',
	call_id TEXT NOT NULL DEFAULT '',
	parent_call_id TEXT NOT NULL DEFAULT '',
	specialist_id TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	input_tokens INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	max_context_tokens INTEGER NOT NULL DEFAULT 0,
	payload_json TEXT NOT NULL,
	redacted BOOLEAN NOT NULL DEFAULT TRUE,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS llm_requests_message_idx ON llm_requests(session_id, message_id, created_at);
CREATE INDEX IF NOT EXISTS llm_requests_parent_user_message_idx ON llm_requests(session_id, parent_user_message_id, created_at);
CREATE INDEX IF NOT EXISTS llm_requests_user_idx ON llm_requests(user_id, created_at);
`)
	return err
}

func (s *sqliteLLMRequestStore) AppendLLMRequest(ctx context.Context, req persistence.LLMRequest) error {
	if err := s.Init(ctx); err != nil {
		return err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = uuid.NewString()
	}
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	payload := string(req.Payload)
	if strings.TrimSpace(payload) == "" {
		payload = "{}"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO llm_requests (
	id, session_id, user_id, run_id, message_id, parent_user_message_id,
	call_id, parent_call_id, specialist_id, provider, model, input_tokens,
	output_tokens, max_context_tokens, payload_json, redacted, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, req.SessionID, nullableInt64Value(req.UserID), req.RunID, req.MessageID,
		req.ParentUserMessageID, req.CallID, req.ParentCallID, req.SpecialistID,
		req.Provider, req.Model, req.InputTokens, req.OutputTokens, req.MaxContextTokens,
		payload, req.Redacted, createdAt,
	)
	return err
}

func (s *sqliteLLMRequestStore) ListLLMRequestsForMessage(ctx context.Context, userID *int64, sessionID string, messageID string) ([]persistence.LLMRequest, error) {
	if s.chat != nil {
		if _, err := s.chat.GetSession(ctx, userID, sessionID); err != nil {
			return nil, err
		}
	}
	if err := s.Init(ctx); err != nil {
		return nil, err
	}
	out, err := s.queryLLMRequests(ctx, `SELECT id, session_id, user_id, run_id, message_id,
	parent_user_message_id, call_id, parent_call_id, specialist_id, provider, model,
	input_tokens, output_tokens, max_context_tokens, payload_json, redacted, created_at
	FROM llm_requests WHERE session_id = ? AND message_id = ? ORDER BY created_at ASC, id ASC`, sessionID, messageID)
	if err != nil || len(out) > 0 {
		return out, err
	}
	parentUserMessageID, ok, err := parentUserMessageIDForAssistantMessage(ctx, s.chat, userID, sessionID, messageID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return out, nil
	}
	return s.queryLLMRequests(ctx, `SELECT id, session_id, user_id, run_id, message_id,
	parent_user_message_id, call_id, parent_call_id, specialist_id, provider, model,
	input_tokens, output_tokens, max_context_tokens, payload_json, redacted, created_at
	FROM llm_requests WHERE session_id = ? AND message_id = '' AND parent_user_message_id = ? ORDER BY created_at ASC, id ASC`, sessionID, parentUserMessageID)
}

func (s *sqliteLLMRequestStore) queryLLMRequests(ctx context.Context, query string, args ...any) ([]persistence.LLMRequest, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []persistence.LLMRequest{}
	for rows.Next() {
		req, err := scanSQLiteLLMRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (s *sqliteLLMRequestStore) GetLLMRequest(ctx context.Context, userID *int64, id string) (persistence.LLMRequest, error) {
	if err := s.Init(ctx); err != nil {
		return persistence.LLMRequest{}, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, session_id, user_id, run_id, message_id,
	parent_user_message_id, call_id, parent_call_id, specialist_id, provider, model,
	input_tokens, output_tokens, max_context_tokens, payload_json, redacted, created_at
	FROM llm_requests WHERE id = ?`, id)
	req, err := scanSQLiteLLMRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return persistence.LLMRequest{}, persistence.ErrNotFound
		}
		return persistence.LLMRequest{}, err
	}
	if s.chat != nil {
		if _, err := s.chat.GetSession(ctx, userID, req.SessionID); err != nil {
			return persistence.LLMRequest{}, err
		}
	}
	return req, nil
}

func (s *sqliteLLMRequestStore) DeleteSessionLLMRequests(ctx context.Context, userID *int64, sessionID string) error {
	if s.chat != nil {
		if _, err := s.chat.GetSession(ctx, userID, sessionID); err != nil {
			return err
		}
	}
	if err := s.Init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM llm_requests WHERE session_id = ?`, sessionID)
	return err
}

func scanSQLiteLLMRequest(scanner interface{ Scan(dest ...any) error }) (persistence.LLMRequest, error) {
	var req persistence.LLMRequest
	var uid sql.NullInt64
	var payload string
	var createdAt sqliteTime
	if err := scanner.Scan(
		&req.ID, &req.SessionID, &uid, &req.RunID, &req.MessageID,
		&req.ParentUserMessageID, &req.CallID, &req.ParentCallID, &req.SpecialistID,
		&req.Provider, &req.Model, &req.InputTokens, &req.OutputTokens,
		&req.MaxContextTokens, &payload, &req.Redacted, &createdAt,
	); err != nil {
		return persistence.LLMRequest{}, err
	}
	req.UserID = int64PtrFromNull(uid)
	req.Payload = []byte(payload)
	req.CreatedAt = createdAt.Time
	return req, nil
}
