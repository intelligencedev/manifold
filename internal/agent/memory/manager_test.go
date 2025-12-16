package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"manifold/internal/llm"
	"manifold/internal/persistence"
)

type stubLLM struct {
	response string
}

func (s *stubLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: s.response}, nil
}

func (s *stubLLM) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	return nil
}

// stubChatStore is a minimal in-memory ChatStore for testing without import cycles.
type stubChatStore struct {
	mu       sync.Mutex
	sessions map[string]*persistence.ChatSession
	messages map[string][]persistence.ChatMessage
}

func newStubChatStore() *stubChatStore {
	return &stubChatStore{
		sessions: make(map[string]*persistence.ChatSession),
		messages: make(map[string][]persistence.ChatMessage),
	}
}

func (s *stubChatStore) Init(ctx context.Context) error { return nil }

func (s *stubChatStore) EnsureSession(ctx context.Context, userID *int64, id string, name string) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		return *sess, nil
	}
	sess := &persistence.ChatSession{ID: id, Name: name, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.sessions[id] = sess
	s.messages[id] = nil
	return *sess, nil
}

func (s *stubChatStore) ListSessions(ctx context.Context, userID *int64) ([]persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []persistence.ChatSession
	for _, sess := range s.sessions {
		result = append(result, *sess)
	}
	return result, nil
}

func (s *stubChatStore) GetSession(ctx context.Context, userID *int64, id string) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		return *sess, nil
	}
	return persistence.ChatSession{}, nil
}

func (s *stubChatStore) CreateSession(ctx context.Context, userID *int64, name string) (persistence.ChatSession, error) {
	return s.EnsureSession(ctx, userID, name, name)
}

func (s *stubChatStore) RenameSession(ctx context.Context, userID *int64, id, name string) (persistence.ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Name = name
		return *sess, nil
	}
	return persistence.ChatSession{}, nil
}

func (s *stubChatStore) DeleteSession(ctx context.Context, userID *int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	delete(s.messages, id)
	return nil
}

func (s *stubChatStore) ListMessages(ctx context.Context, userID *int64, sessionID string, limit int) ([]persistence.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[sessionID]
	if limit > 0 && len(msgs) > limit {
		return msgs[len(msgs)-limit:], nil
	}
	return msgs, nil
}

func (s *stubChatStore) AppendMessages(ctx context.Context, userID *int64, sessionID string, messages []persistence.ChatMessage, preview string, model string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[sessionID] = append(s.messages[sessionID], messages...)
	if sess, ok := s.sessions[sessionID]; ok {
		sess.UpdatedAt = time.Now()
	}
	return nil
}

func (s *stubChatStore) UpdateSummary(ctx context.Context, userID *int64, sessionID string, summary string, summarizedCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		sess.Summary = summary
		sess.SummarizedCount = summarizedCount
	}
	return nil
}

func TestManagerBuildContextWithSummary(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()

	if _, err := store.EnsureSession(ctx, nil, "sess", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	now := time.Now().UTC()
	turns := []struct {
		user      string
		assistant string
	}{
		{"u1", "a1"},
		{"u2", "a2"},
		{"u3", "a3"},
	}
	for i, turn := range turns {
		messages := []persistence.ChatMessage{
			{Role: "user", Content: turn.user, CreatedAt: now.Add(time.Duration(i*2) * time.Second)},
			{Role: "assistant", Content: turn.assistant, CreatedAt: now.Add(time.Duration(i*2+1) * time.Second)},
		}
		if err := store.AppendMessages(ctx, nil, "sess", messages, turn.assistant, "model"); err != nil {
			t.Fatalf("AppendMessages: %v", err)
		}
	}

	manager := NewManager(store, &stubLLM{response: "summary"}, Config{Enabled: true, Threshold: 4, KeepLast: 2, SummaryModel: "stub"})
	history, err := manager.BuildContext(ctx, nil, "sess")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 messages (summary + 2 turns), got %d", len(history))
	}
	if history[0].Role != "system" || history[0].Content == "" {
		t.Fatalf("expected system summary message, got %#v", history[0])
	}
	if history[1].Content != "u3" || history[2].Content != "a3" {
		t.Fatalf("unexpected tail messages: %#v", history[1:])
	}

	session, err := store.GetSession(ctx, nil, "sess")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Summary == "" {
		t.Fatalf("expected summary to be stored")
	}
	if session.SummarizedCount != 4 {
		t.Fatalf("expected summarized count 4, got %d", session.SummarizedCount)
	}
}
