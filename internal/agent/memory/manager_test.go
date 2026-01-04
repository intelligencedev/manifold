package memory

import (
	"context"
	"encoding/json"
	"strings"
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

type stubCompactor struct {
	item  llm.CompactionItem
	calls int
}

func (s *stubCompactor) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	return llm.Message{Role: "assistant", Content: "unused"}, nil
}

func (s *stubCompactor) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
	return nil
}

func (s *stubCompactor) Compact(ctx context.Context, msgs []llm.Message, model string, previous *llm.CompactionItem) (*llm.CompactionItem, error) {
	s.calls++
	return &s.item, nil
}

type recordingLLM struct {
	response string
	lastMsgs []llm.Message
}

func (r *recordingLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	r.lastMsgs = append([]llm.Message(nil), msgs...)
	return llm.Message{Role: "assistant", Content: r.response}, nil
}

func (r *recordingLLM) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
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
	// Use longer content to ensure proper token counting
	turns := []struct {
		user      string
		assistant string
	}{
		{"user message one with some content", "assistant message one with some content"},
		{"user message two with some content", "assistant message two with some content"},
		{"user message three with some content", "assistant message three with some content"},
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

	// With 6 messages at ~10 tokens each = ~60 tokens
	// Context window = 50, reserve buffer = 5, budget = 45
	// This should trigger summarization
	manager := NewManager(store, &stubLLM{response: "summary"}, Config{
		Enabled:             true,
		ReserveBufferTokens: 5,
		MinKeepLastMessages: 2,
		ContextWindowTokens: 50, // Smaller than total tokens to trigger summarization
		SummaryModel:        "stub",
	})
	history, summaryResult, err := manager.BuildContext(ctx, nil, "sess")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 messages (summary + 2 turns), got %d: %+v", len(history), history)
	}
	if history[0].Role != "system" || history[0].Content == "" {
		t.Fatalf("expected system summary message, got %#v", history[0])
	}
	if history[1].Content != "user message three with some content" || history[2].Content != "assistant message three with some content" {
		t.Fatalf("unexpected tail messages: %#v", history[1:])
	}
	if summaryResult == nil || !summaryResult.Triggered {
		t.Fatalf("expected summaryResult.Triggered to be true")
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

func TestManagerBuildContextWithCompaction(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()

	if _, err := store.EnsureSession(ctx, nil, "sess", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	now := time.Now().UTC()
	// Use longer content to ensure proper token counting
	for i := 0; i < 3; i++ {
		messages := []persistence.ChatMessage{
			{Role: "user", Content: "user message with enough content to trigger summarization", CreatedAt: now.Add(time.Duration(i*2) * time.Second)},
			{Role: "assistant", Content: "assistant message with enough content to trigger summarization", CreatedAt: now.Add(time.Duration(i*2+1) * time.Second)},
		}
		if err := store.AppendMessages(ctx, nil, "sess", messages, "a", "model"); err != nil {
			t.Fatalf("AppendMessages: %v", err)
		}
	}

	compactor := &stubCompactor{item: llm.CompactionItem{EncryptedContent: "enc"}}
	// With 6 messages at ~15 tokens each = ~90 tokens
	// Context window = 50, reserve buffer = 5, budget = 45
	// This should trigger summarization
	manager := NewManager(store, compactor, Config{
		Enabled:                true,
		ReserveBufferTokens:    5,
		MinKeepLastMessages:    2,
		ContextWindowTokens:    50, // Smaller than total tokens to trigger summarization
		SummaryModel:           "stub",
		UseResponsesCompaction: true,
	})

	history, summaryResult, err := manager.BuildContext(ctx, nil, "sess")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 messages (compaction + 2 turns), got %d", len(history))
	}
	if history[0].Compaction == nil || history[0].Compaction.EncryptedContent != "enc" {
		t.Fatalf("expected compaction item in history, got %#v", history[0])
	}
	if summaryResult == nil || !summaryResult.Triggered {
		t.Fatalf("expected summaryResult.Triggered to be true")
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
	if compactor.calls == 0 {
		t.Fatalf("expected compaction provider to be called")
	}
}

func TestSummarizeChunkFormatsToolMessages(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()
	recorder := &recordingLLM{response: "summary"}
	manager := NewManager(store, recorder, Config{
		Enabled:                true,
		SummaryModel:           "stub",
		MaxSummaryChunkTokens:  40,
		UseResponsesCompaction: false,
	})

	assistantPayload := map[string]any{
		"content": "assistant message",
		"tool_calls": []llm.ToolCall{
			{Name: "run_cli", Args: json.RawMessage(`{"cmd":"ls -la"}`), ID: "call_1"},
		},
	}
	assistantRaw, err := json.Marshal(assistantPayload)
	if err != nil {
		t.Fatalf("marshal assistant payload: %v", err)
	}

	toolPayload := map[string]any{
		"content": strings.Repeat("A", 30) + "MIDDLE" + strings.Repeat("B", 30),
		"tool_id": "call_1",
	}
	toolRaw, err := json.Marshal(toolPayload)
	if err != nil {
		t.Fatalf("marshal tool payload: %v", err)
	}

	_, err = manager.summarizeChunk(ctx, "", []persistence.ChatMessage{
		{Role: "assistant", Content: string(assistantRaw)},
		{Role: "tool", Content: string(toolRaw)},
	})
	if err != nil {
		t.Fatalf("summarizeChunk: %v", err)
	}
	if len(recorder.lastMsgs) < 2 {
		t.Fatalf("expected summarizer prompt messages, got %d", len(recorder.lastMsgs))
	}
	prompt := recorder.lastMsgs[1].Content
	if !strings.Contains(prompt, "Tool calls: run_cli") {
		t.Fatalf("expected tool call summary in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Tool ID: call_1") {
		t.Fatalf("expected tool id in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "[TRUNCATED]") {
		t.Fatalf("expected truncation marker in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "AAAAA") || !strings.Contains(prompt, "BBBBB") {
		t.Fatalf("expected head and tail content in prompt, got %q", prompt)
	}
}
