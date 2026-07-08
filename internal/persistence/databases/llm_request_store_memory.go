package databases

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"manifold/internal/persistence"
)

func NewMemoryLLMRequestStore(chat persistence.ChatStore) persistence.LLMRequestStore {
	return &memLLMRequestStore{chat: chat, records: map[string]persistence.LLMRequest{}}
}

type memLLMRequestStore struct {
	mu      sync.RWMutex
	chat    persistence.ChatStore
	records map[string]persistence.LLMRequest
}

func (s *memLLMRequestStore) Init(context.Context) error { return nil }

func (s *memLLMRequestStore) AppendLLMRequest(_ context.Context, req persistence.LLMRequest) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return persistence.ErrNotFound
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = uuid.NewString()
	}
	req.ID = id
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	if req.Payload == nil {
		req.Payload = []byte("{}")
	}
	s.mu.Lock()
	s.records[id] = cloneLLMRequest(req)
	s.mu.Unlock()
	return nil
}

func (s *memLLMRequestStore) ListLLMRequestsForMessage(ctx context.Context, userID *int64, sessionID string, messageID string) ([]persistence.LLMRequest, error) {
	if s.chat != nil {
		if _, err := s.chat.GetSession(ctx, userID, sessionID); err != nil {
			return nil, err
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []persistence.LLMRequest{}
	for _, req := range s.records {
		if req.SessionID == sessionID && req.MessageID == messageID {
			out = append(out, cloneLLMRequest(req))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *memLLMRequestStore) GetLLMRequest(ctx context.Context, userID *int64, id string) (persistence.LLMRequest, error) {
	s.mu.RLock()
	req, ok := s.records[id]
	s.mu.RUnlock()
	if !ok {
		return persistence.LLMRequest{}, persistence.ErrNotFound
	}
	if s.chat != nil {
		if _, err := s.chat.GetSession(ctx, userID, req.SessionID); err != nil {
			return persistence.LLMRequest{}, err
		}
	}
	return cloneLLMRequest(req), nil
}

func (s *memLLMRequestStore) DeleteSessionLLMRequests(ctx context.Context, userID *int64, sessionID string) error {
	if s.chat != nil {
		if _, err := s.chat.GetSession(ctx, userID, sessionID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, req := range s.records {
		if req.SessionID == sessionID {
			delete(s.records, id)
		}
	}
	return nil
}

func cloneLLMRequest(req persistence.LLMRequest) persistence.LLMRequest {
	clone := req
	clone.UserID = cloneInt64Ptr(req.UserID)
	if req.Payload != nil {
		clone.Payload = append([]byte(nil), req.Payload...)
	}
	return clone
}

func cloneInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}
