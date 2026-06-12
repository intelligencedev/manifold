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

func newChatSession(id string, userID *int64, name string, kind string, now time.Time) persistence.ChatSession {
	return persistence.ChatSession{
		ID:                    id,
		Name:                  name,
		Kind:                  kind,
		UserID:                copyUserID(userID),
		CreatedAt:             now,
		UpdatedAt:             now,
		MemoryEnabled:         false,
		EvolvingMemoryEnabled: false,
		BeliefMemoryEnabled:   false,
	}
}

func copyUserID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func (s *memChatStore) sessionWithMessageCountLocked(sess persistence.ChatSession) persistence.ChatSession {
	sess.MessageCount = len(s.messages[sess.ID])
	return sess
}

func normalizeChatSessionKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return persistence.ChatSessionKindChat
	}
	return kind
}

func (s *memChatStore) EnsureSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	return s.EnsureSessionKind(ctx, userID, id, name, persistence.ChatSessionKindChat)
}

func (s *memChatStore) EnsureSessionKind(ctx context.Context, userID *int64, id, name, kind string) (persistence.ChatSession, error) {
	if strings.TrimSpace(id) == "" {
		return persistence.ChatSession{}, errors.New("id required")
	}
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	kind = normalizeChatSessionKind(kind)
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		if !hasAccess(userID, sess.UserID) {
			return persistence.ChatSession{}, persistence.ErrForbidden
		}
		if sess.Kind == "" {
			sess.Kind = persistence.ChatSessionKindChat
			s.sessions[id] = sess
		}
		return s.sessionWithMessageCountLocked(sess), nil
	}
	now := time.Now().UTC()
	sess := newChatSession(id, userID, name, kind, now)
	s.sessions[id] = sess
	s.messages[id] = nil
	return s.sessionWithMessageCountLocked(sess), nil
}

func (s *memChatStore) ListSessions(ctx context.Context, userID *int64) ([]persistence.ChatSession, error) {
	return s.ListSessionsByKind(ctx, userID, persistence.ChatSessionKindChat)
}

func (s *memChatStore) ListSessionsByKind(ctx context.Context, userID *int64, kind string) ([]persistence.ChatSession, error) {
	kind = normalizeChatSessionKind(kind)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]persistence.ChatSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if !hasAccess(userID, sess.UserID) {
			continue
		}
		if normalizeChatSessionKind(sess.Kind) != kind {
			continue
		}
		out = append(out, s.sessionWithMessageCountLocked(sess))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
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
	return s.sessionWithMessageCountLocked(sess), nil
}

func (s *memChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	return s.CreateSessionKind(ctx, userID, name, persistence.ChatSessionKindChat)
}

func (s *memChatStore) CreateSessionKind(ctx context.Context, userID *int64, name, kind string) (persistence.ChatSession, error) {
	if strings.TrimSpace(name) == "" {
		name = "New Chat"
	}
	kind = normalizeChatSessionKind(kind)
	s.mu.Lock()
	defer s.mu.Unlock()
	id := uuid.NewString()
	now := time.Now().UTC()
	sess := newChatSession(id, userID, name, kind, now)
	s.sessions[id] = sess
	s.messages[id] = nil
	return s.sessionWithMessageCountLocked(sess), nil
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
	return s.sessionWithMessageCountLocked(sess), nil
}

func (s *memChatStore) SetSessionProject(ctx context.Context, userID *int64, id, projectID string) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	sess.ProjectID = strings.TrimSpace(projectID)
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[id] = sess
	return s.sessionWithMessageCountLocked(sess), nil
}

func (s *memChatStore) SetSessionMemorySettings(ctx context.Context, userID *int64, id string, memoryEnabled bool, evolvingMemoryEnabled bool, beliefMemoryEnabled bool) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	sess.MemoryEnabled = memoryEnabled
	sess.EvolvingMemoryEnabled = memoryEnabled
	sess.BeliefMemoryEnabled = memoryEnabled
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[id] = sess
	return s.sessionWithMessageCountLocked(sess), nil
}

func (s *memChatStore) SetSessionActiveTarget(ctx context.Context, userID *int64, id string, activeSpecialist string, activeTeam string) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	sess.ActiveSpecialist = strings.TrimSpace(activeSpecialist)
	sess.ActiveTeam = strings.TrimSpace(activeTeam)
	s.sessions[id] = sess
	return s.sessionWithMessageCountLocked(sess), nil
}

func (s *memChatStore) SetSessionPinned(ctx context.Context, userID *int64, id string, pinned bool) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return persistence.ChatSession{}, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ChatSession{}, persistence.ErrForbidden
	}
	sess.Pinned = pinned
	s.sessions[id] = sess
	return s.sessionWithMessageCountLocked(sess), nil
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

func (s *memChatStore) DeleteMessageWithRelated(ctx context.Context, userID *int64, sessionID string, messageID string, relatedMessageIDs []string, resetSummary bool) error {
	if strings.TrimSpace(messageID) == "" {
		return persistence.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, msgs, err := s.mustAccessSessionLocked(userID, sessionID)
	if err != nil {
		return err
	}
	deleteIDs := uniqueChatMessageIDs(messageID, relatedMessageIDs)
	msgSet := make(map[string]struct{}, len(deleteIDs))
	for _, id := range deleteIDs {
		msgSet[id] = struct{}{}
	}
	filtered := msgs[:0]
	deletedTarget := false
	for _, msg := range msgs {
		if _, ok := msgSet[msg.ID]; ok {
			if msg.ID == messageID {
				deletedTarget = true
			}
			continue
		}
		filtered = append(filtered, msg)
	}
	if !deletedTarget {
		return persistence.ErrNotFound
	}
	s.messages[sessionID] = filtered
	s.finalizeDeleteLocked(sessionID, sess, filtered, resetSummary)
	return nil
}

func (s *memChatStore) DeleteMessage(ctx context.Context, userID *int64, sessionID string, messageID string) error {
	if strings.TrimSpace(messageID) == "" {
		return persistence.ErrNotFound
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
	msgs := s.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return persistence.ErrNotFound
	}
	msgs = append(msgs[:idx], msgs[idx+1:]...)
	s.messages[sessionID] = msgs

	preview := ""
	if len(msgs) > 0 {
		preview = snippetForPreview(msgs[len(msgs)-1].Content)
	}
	sess.LastMessagePreview = preview
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[sessionID] = sess
	return nil
}

func (s *memChatStore) DeleteMessagesAfterWithRelated(ctx context.Context, req persistence.ChatDeleteAfterRequest) error {
	if strings.TrimSpace(req.MessageID) == "" {
		return persistence.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, msgs, err := s.mustAccessSessionLocked(req.UserID, req.SessionID)
	if err != nil {
		return err
	}
	idx := -1
	for i, msg := range msgs {
		if msg.ID == req.MessageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return persistence.ErrNotFound
	}

	cut := idx + 1
	if req.Inclusive {
		cut = idx
	}
	if cut < 0 {
		cut = 0
	}
	if cut > len(msgs) {
		cut = len(msgs)
	}
	filtered := append([]persistence.ChatMessage(nil), msgs[:cut]...)
	if len(req.RelatedMessageIDs) > 0 {
		relatedSet := make(map[string]struct{}, len(req.RelatedMessageIDs))
		for _, id := range req.RelatedMessageIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				relatedSet[id] = struct{}{}
			}
		}
		remaining := filtered[:0]
		for _, msg := range filtered {
			if _, ok := relatedSet[msg.ID]; ok {
				continue
			}
			remaining = append(remaining, msg)
		}
		filtered = remaining
	}
	s.messages[req.SessionID] = filtered
	s.finalizeDeleteLocked(req.SessionID, sess, filtered, req.ResetSummary)
	return nil
}

func (s *memChatStore) DeleteMessagesAfter(ctx context.Context, userID *int64, sessionID string, messageID string, inclusive bool) error {
	if strings.TrimSpace(messageID) == "" {
		return persistence.ErrNotFound
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
	msgs := s.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return persistence.ErrNotFound
	}
	cut := idx + 1
	if inclusive {
		cut = idx
	}
	if cut < 0 {
		cut = 0
	}
	if cut > len(msgs) {
		cut = len(msgs)
	}
	msgs = msgs[:cut]
	s.messages[sessionID] = msgs

	preview := ""
	if len(msgs) > 0 {
		preview = snippetForPreview(msgs[len(msgs)-1].Content)
	}
	sess.LastMessagePreview = preview
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[sessionID] = sess
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
	return s.appendMessages(chatAppendMessagesRequest{ctx: ctx, userID: userID, sessionID: sessionID, messages: messages, preview: preview, model: model})
}

func (s *memChatStore) AppendMessagesOnce(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	return s.appendMessages(chatAppendMessagesRequest{ctx: ctx, userID: userID, sessionID: sessionID, messages: messages, preview: preview, model: model, skipExisting: true})
}

func (s *memChatStore) appendMessages(req chatAppendMessagesRequest) error {
	log.Info().Str("session_id", req.sessionID).Int("count", len(req.messages)).Msg("mem_store_append_messages")
	if len(req.messages) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[req.sessionID]
	if !ok {
		return persistence.ErrNotFound
	}
	if !hasAccess(req.userID, sess.UserID) {
		return persistence.ErrForbidden
	}
	existing := map[string]struct{}{}
	if req.skipExisting {
		for _, message := range s.messages[req.sessionID] {
			if strings.TrimSpace(message.ID) == "" {
				continue
			}
			existing[message.ID] = struct{}{}
		}
	}
	out := make([]persistence.ChatMessage, 0, len(req.messages))
	for i := range req.messages {
		if req.messages[i].ID == "" {
			req.messages[i].ID = uuid.NewString()
		}
		if _, ok := existing[req.messages[i].ID]; ok {
			continue
		}
		if req.messages[i].SessionID == "" {
			req.messages[i].SessionID = req.sessionID
		}
		if req.messages[i].CreatedAt.IsZero() {
			req.messages[i].CreatedAt = time.Now().UTC()
		}
		out = append(out, req.messages[i])
	}
	if len(out) == 0 && req.skipExisting {
		return nil
	}
	s.messages[req.sessionID] = append(s.messages[req.sessionID], out...)
	sess.UpdatedAt = time.Now().UTC()
	sess.LastMessagePreview = req.preview
	if strings.TrimSpace(req.model) != "" {
		sess.Model = req.model
	}
	s.sessions[req.sessionID] = sess
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

func (s *memChatStore) mustAccessSessionLocked(userID *int64, sessionID string) (persistence.ChatSession, []persistence.ChatMessage, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return persistence.ChatSession{}, nil, persistence.ErrNotFound
	}
	if !hasAccess(userID, sess.UserID) {
		return persistence.ChatSession{}, nil, persistence.ErrForbidden
	}
	return sess, s.messages[sessionID], nil
}

func (s *memChatStore) finalizeDeleteLocked(sessionID string, sess persistence.ChatSession, msgs []persistence.ChatMessage, resetSummary bool) {
	preview := ""
	if len(msgs) > 0 {
		preview = snippetForPreview(msgs[len(msgs)-1].Content)
	}
	if resetSummary {
		sess.Summary = ""
		sess.SummarizedCount = 0
	}
	sess.LastMessagePreview = preview
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[sessionID] = sess
}

func uniqueChatMessageIDs(primaryID string, extraIDs []string) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(extraIDs)+1)
	for _, id := range append([]string{primaryID}, extraIDs...) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
