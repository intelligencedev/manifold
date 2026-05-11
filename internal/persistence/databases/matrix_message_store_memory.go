package databases

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"manifold/internal/persistence"
)

func NewMatrixMessageStore(pool *pgxpool.Pool) persistence.MatrixMessageStore {
	if pool == nil {
		return newMemoryMatrixMessageStore()
	}
	return NewPostgresMatrixMessageStore(pool)
}

type memMatrixMessageStore struct {
	mu      sync.RWMutex
	nextID  int64
	byRoom  map[string][]persistence.MatrixMessage
	byEvent map[string]persistence.MatrixMessage
}

func newMemoryMatrixMessageStore() persistence.MatrixMessageStore {
	return &memMatrixMessageStore{
		nextID:  1,
		byRoom:  map[string][]persistence.MatrixMessage{},
		byEvent: map[string]persistence.MatrixMessage{},
	}
}

func (s *memMatrixMessageStore) Init(context.Context) error { return nil }

func (s *memMatrixMessageStore) Append(_ context.Context, message persistence.MatrixMessage, maxMessages int) (persistence.MatrixMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	message.RoomID = strings.TrimSpace(message.RoomID)
	message.EventID = strings.TrimSpace(message.EventID)
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	if message.EventID != "" {
		if existing, ok := s.byEvent[message.EventID]; ok {
			return existing, nil
		}
	}
	message.ID = s.nextID
	s.nextID++
	s.byRoom[message.RoomID] = append(s.byRoom[message.RoomID], message)
	if message.EventID != "" {
		s.byEvent[message.EventID] = message
	}
	if maxMessages > 0 {
		messages := s.byRoom[message.RoomID]
		if len(messages) > maxMessages {
			trimmed := messages[len(messages)-maxMessages:]
			for _, pruned := range messages[:len(messages)-maxMessages] {
				if pruned.EventID != "" {
					delete(s.byEvent, pruned.EventID)
				}
			}
			s.byRoom[message.RoomID] = append([]persistence.MatrixMessage(nil), trimmed...)
		}
	}
	return message, nil
}

func (s *memMatrixMessageStore) ListByRoom(_ context.Context, roomID string, limit int, beforeID int64) ([]persistence.MatrixMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages := s.byRoom[strings.TrimSpace(roomID)]
	out := make([]persistence.MatrixMessage, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if beforeID > 0 && msg.ID >= beforeID {
			continue
		}
		out = append(out, msg)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if out == nil {
		out = []persistence.MatrixMessage{}
	}
	return out, nil
}

func (s *memMatrixMessageStore) Prune(_ context.Context, roomID string, maxMessages int) error {
	if maxMessages <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	roomID = strings.TrimSpace(roomID)
	messages := s.byRoom[roomID]
	if len(messages) <= maxMessages {
		return nil
	}
	trimmed := messages[len(messages)-maxMessages:]
	for _, pruned := range messages[:len(messages)-maxMessages] {
		if pruned.EventID != "" {
			delete(s.byEvent, pruned.EventID)
		}
	}
	s.byRoom[roomID] = append([]persistence.MatrixMessage(nil), trimmed...)
	return nil
}

func (s *memMatrixMessageStore) RoomStats(_ context.Context, roomID string) (persistence.MatrixRoomStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roomID = strings.TrimSpace(roomID)
	messages := s.byRoom[roomID]
	stats := persistence.MatrixRoomStats{RoomID: roomID, MessageCount: len(messages)}
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		stats.LastActivityAt = last.CreatedAt
		stats.LastSender = last.Sender
	}
	return stats, nil
}

func (s *memMatrixMessageStore) Close() {}

func sortMatrixMessagesNewestFirst(messages []persistence.MatrixMessage) {
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID > messages[j].ID
		}
		return messages[i].CreatedAt.After(messages[j].CreatedAt)
	})
}
