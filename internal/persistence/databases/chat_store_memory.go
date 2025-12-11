package databases

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/persistence"
)

func newMemoryChatStore() persistence.ChatStore {
	return &memChatStore{
		sessions: map[string]persistence.ChatSession{},
		messages: map[string][]persistence.ChatMessage{},
	}
}

type memChatStore struct {
	mu       sync.RWMutex
	sessions map[string]persistence.ChatSession
	messages map[string][]persistence.ChatMessage
}

func (s *memChatStore) Init(ctx context.Context) error { return nil }

func copyUserID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func (s *memChatStore) EnsureSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(id) == "" {
		return persistence.ChatSession{}, errors.New("id required")
	}
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		if !hasAccess(userID, sess.UserID) {
			return persistence.ChatSession{}, persistence.ErrForbidden
		}
		return sess, nil
	}
	now := time.Now().UTC()
	sess := persistence.ChatSession{ID: id, Name: name, UserID: copyUserID(userID), CreatedAt: now, UpdatedAt: now}
	s.sessions[id] = sess
	s.messages[id] = nil
	return sess, nil
}

func (s *memChatStore) ListSessions(ctx context.Context, userID *int64) ([]persistence.ChatSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]persistence.ChatSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if !hasAccess(userID, sess.UserID) {
			continue
		}
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *memChatStore) GetSession(ctx context.Context, userID *int64, id string) (persistence.ChatSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	return sess, nil
}

func (s *memChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := uuid.NewString()
	now := time.Now().UTC()
	sess := persistence.ChatSession{ID: id, Name: name, UserID: copyUserID(userID), CreatedAt: now, UpdatedAt: now}
	s.sessions[id] = sess
	s.messages[id] = nil
	return sess, nil
}

func (s *memChatStore) RenameSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		return persistence.ChatSession{}, errors.New("name required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	sess.Name = name
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[id] = sess
	return sess, nil
}

func (s *memChatStore) DeleteSession(ctx context.Context, userID *int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ErrForbidden
	}
	delete(s.sessions, id)
	delete(s.messages, id)
	return nil
}

func (s *memChatStore) ListMessages(ctx context.Context, userID *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return nil, persistence.ErrForbidden
	}
	msgs := s.messages[sessionID]
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	log.Info().Str("session_id", sessionID).Int("count", len(msgs)).Msg("mem_store_list_messages")
	out := make([]persistence.ChatMessage, len(msgs))
	copy(out, msgs)
	return out, nil
}

func (s *memChatStore) AppendMessages(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	log.Info().Str("session_id", sessionID).Int("count", len(messages)).Msg("mem_store_append_messages")
	if len(messages) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ErrForbidden
	}
	for i := range messages {
		if messages[i].ID == "" {
			messages[i].ID = uuid.NewString()
		}
		if messages[i].SessionID == "" {
			messages[i].SessionID = sessionID
		}
		if messages[i].CreatedAt.IsZero() {
			messages[i].CreatedAt = time.Now().UTC()
		}
	}
	s.messages[sessionID] = append(s.messages[sessionID], messages...)
	sess.UpdatedAt = time.Now().UTC()
	sess.LastMessagePreview = preview
	if strings.TrimSpace(model) != "" {
		sess.Model = model
	}
	s.sessions[sessionID] = sess
	return nil
}

func (s *memChatStore) UpdateSummary(ctx context.Context, userID *int64, sessionID string, summary string, summarizedCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ErrForbidden
	}
	sess.Summary = summary
	sess.SummarizedCount = summarizedCount
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[sessionID] = sess
	return nil
}
