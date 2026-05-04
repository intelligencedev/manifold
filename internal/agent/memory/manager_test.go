package memory

import (
	"context"
	"encoding/json"
	"errors"
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
	err   error
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
	if s.err != nil {
		return nil, s.err
	}
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

type limitEnforcingLLM struct {
	response        string
	maxPromptTokens int
	calls           int
	maxSeenTokens   int
}

func (l *limitEnforcingLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string) (llm.Message, error) {
	l.calls++
	tokens := estimateMessagesTokens(msgs)
	if tokens > l.maxSeenTokens {
		l.maxSeenTokens = tokens
	}
	if l.maxPromptTokens > 0 && tokens > l.maxPromptTokens {
		return llm.Message{}, errors.New("prompt exceeds test limit")
	}
	return llm.Message{Role: "assistant", Content: l.response}, nil
}

func (l *limitEnforcingLLM) ChatStream(ctx context.Context, msgs []llm.Message, tools []llm.ToolSchema, model string, h llm.StreamHandler) error {
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

func (s *stubChatStore) DeleteMessage(ctx context.Context, userID *int64, sessionID string, messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	msgs = append(msgs[:idx], msgs[idx+1:]...)
	s.messages[sessionID] = msgs
	if sess, ok := s.sessions[sessionID]; ok {
		sess.UpdatedAt = time.Now()
	}
	return nil
}

func (s *stubChatStore) DeleteMessagesAfter(ctx context.Context, userID *int64, sessionID string, messageID string, inclusive bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[sessionID]
	idx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
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
	s.messages[sessionID] = msgs[:cut]
	if sess, ok := s.sessions[sessionID]; ok {
		sess.UpdatedAt = time.Now()
	}
	return nil
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
	history, summaryResult, err := manager.BuildContextForProvider(ctx, nil, "sess", nil, "", SummaryPolicy{})
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

func TestBuildContextForProvider_DoesNotOrphanToolMessagesInTail(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()

	if _, err := store.EnsureSession(ctx, nil, "sess", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	// Persist an assistant tool call and its tool response. The manager tail-selection
	// logic can cap to the last message and accidentally start the returned history
	// on the tool message, which breaks providers like Anthropic.
	call := llm.ToolCall{Name: "lookup", ID: "call-1", Args: json.RawMessage(`{"q":"x"}`)}
	assistantPayload, _ := json.Marshal(map[string]any{
		"content":    "",
		"tool_calls": []llm.ToolCall{call},
	})
	toolPayload, _ := json.Marshal(map[string]any{
		"content": "{\"ok\":true}",
		"tool_id": "call-1",
	})

	now := time.Now().UTC()
	msgs := []persistence.ChatMessage{
		{Role: "assistant", Content: string(assistantPayload), CreatedAt: now},
		{Role: "tool", Content: string(toolPayload), CreatedAt: now.Add(time.Second)},
	}
	if err := store.AppendMessages(ctx, nil, "sess", msgs, "", "model"); err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	manager := NewManager(store, &stubLLM{response: "unused"}, Config{
		Enabled:             true,
		MinKeepLastMessages: 1,
		MaxKeepLastMessages: 1,
		ContextWindowTokens: 4096,
		ReserveBufferTokens: 32,
		SummaryModel:        "stub",
	})

	history, _, err := manager.BuildContextForProvider(ctx, nil, "sess", nil, "", SummaryPolicy{})
	if err != nil {
		t.Fatalf("BuildContextForProvider: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 messages (assistant tool call + tool result), got %d: %#v", len(history), history)
	}
	if history[0].Role == "tool" {
		t.Fatalf("unexpected orphaned tool message at start of history: %#v", history[0])
	}
	if history[0].Role != "assistant" || len(history[0].ToolCalls) != 1 || history[0].ToolCalls[0].ID != "call-1" {
		t.Fatalf("expected assistant tool call to be preserved, got %#v", history[0])
	}
	if history[1].Role != "tool" || strings.TrimSpace(history[1].ToolID) != "call-1" {
		t.Fatalf("expected tool message with matching ToolID, got %#v", history[1])
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
		Enabled:             true,
		ReserveBufferTokens: 5,
		MinKeepLastMessages: 2,
		ContextWindowTokens: 50, // Smaller than total tokens to trigger summarization
		SummaryModel:        "stub",
	})

	history, summaryResult, err := manager.BuildContextForProvider(ctx, nil, "sess", compactor, "stub", SummaryPolicy{})
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 messages (system rule + compaction), got %d", len(history))
	}
	if history[0].Role != "system" {
		t.Fatalf("expected system continuation rule message, got %#v", history[0])
	}
	if history[1].Compaction == nil || history[1].Compaction.EncryptedContent != "enc" {
		t.Fatalf("expected compaction item in history, got %#v", history[1])
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
	if session.SummarizedCount != 6 {
		t.Fatalf("expected summarized count 6, got %d", session.SummarizedCount)
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
		Enabled:               true,
		SummaryModel:          "stub",
		MaxSummaryChunkTokens: 40,
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
	}, nil, "")
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

func TestSummarizeChunkDisablesUnavailableCompactionEndpoint(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()
	compactor := &stubCompactor{
		err: errors.New(`POST "http://localhost:11434/v1/responses/compact": 404 Not Found {"type":"not_found_error"}`),
	}
	manager := NewManager(store, compactor, Config{
		Enabled:      true,
		SummaryModel: "stub",
	})
	chunk := []persistence.ChatMessage{
		{Role: "user", Content: "remember that local summary providers do not implement compact"},
	}

	summary, err := manager.summarizeChunk(ctx, "", chunk, compactor, "stub")
	if err != nil {
		t.Fatalf("summarizeChunk: %v", err)
	}
	if summary != "unused" {
		t.Fatalf("expected plain summary fallback, got %q", summary)
	}
	if compactor.calls != 1 {
		t.Fatalf("expected one compaction attempt, got %d", compactor.calls)
	}

	summary, err = manager.summarizeChunk(ctx, summary, chunk, compactor, "stub")
	if err != nil {
		t.Fatalf("summarizeChunk after disable: %v", err)
	}
	if summary != "unused" {
		t.Fatalf("expected plain summary after compaction disable, got %q", summary)
	}
	if compactor.calls != 1 {
		t.Fatalf("expected compaction to remain disabled, got %d calls", compactor.calls)
	}
}

func TestSummarizeChunkRecursivelyReducesOversizedExistingSummary(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()
	provider := &limitEnforcingLLM{
		response:        "condensed summary",
		maxPromptTokens: 160,
	}
	manager := NewManager(store, provider, Config{
		Enabled:                      true,
		SummaryModel:                 "stub",
		ContextWindowTokens:          192,
		PlainTextContextWindowTokens: 192,
		ReserveBufferTokens:          32,
		MaxSummaryChunkTokens:        160,
	})

	existing := strings.Repeat("existing summary detail ", 240)
	chunk := []persistence.ChatMessage{
		{Role: "user", Content: "Investigate the latest failure from the summary worker and keep the root cause."},
		{Role: "assistant", Content: "The worker overflowed the summary model context while trying to resummarize prior state."},
	}

	summary, err := manager.summarizeChunk(ctx, existing, chunk, nil, "")
	if err != nil {
		t.Fatalf("summarizeChunk: %v", err)
	}
	if strings.TrimSpace(summary) != "condensed summary" {
		t.Fatalf("unexpected summary: %q", summary)
	}
	if provider.calls < 2 {
		t.Fatalf("expected recursive reduction to require multiple summarizer calls, got %d", provider.calls)
	}
	if provider.maxSeenTokens > provider.maxPromptTokens {
		t.Fatalf("expected each summary prompt to stay within limit, saw %d > %d", provider.maxSeenTokens, provider.maxPromptTokens)
	}
}

func TestPlainTextSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain legacy summary",
			in:   "plain summary",
			want: "plain summary",
		},
		{
			name: "dual summary returns plain",
			in:   `{"compaction":"{\"type\":\"compaction\",\"encrypted_content\":\"enc\"}","plain":"plain fallback"}`,
			want: "plain fallback",
		},
		{
			name: "compaction only returns empty",
			in:   `{"type":"compaction","encrypted_content":"enc"}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := PlainTextSummary(tt.in); got != tt.want {
				t.Fatalf("PlainTextSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildContextForProvider_UsesStricterPlainTextBudgetForNonCompaction(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()

	if _, err := store.EnsureSession(ctx, nil, "sess", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		messages := []persistence.ChatMessage{
			{Role: "user", Content: "user message with enough content to exceed a small plain-text budget", CreatedAt: now.Add(time.Duration(i*2) * time.Second)},
			{Role: "assistant", Content: "assistant message with enough content to exceed a small plain-text budget", CreatedAt: now.Add(time.Duration(i*2+1) * time.Second)},
		}
		if err := store.AppendMessages(ctx, nil, "sess", messages, "a", "model"); err != nil {
			t.Fatalf("AppendMessages: %v", err)
		}
	}

	manager := NewManager(store, &stubLLM{response: "summary"}, Config{
		Enabled:                      true,
		ReserveBufferTokens:          5,
		MinKeepLastMessages:          2,
		ContextWindowTokens:          200,
		PlainTextContextWindowTokens: 50,
		SummaryModel:                 "stub",
	})

	_, summaryResult, err := manager.BuildContextForProvider(ctx, nil, "sess", nil, "target-model", SummaryPolicy{
		TargetContextWindowTokens:    200,
		PlainTextContextWindowTokens: 50,
	})
	if err != nil {
		t.Fatalf("BuildContextForProvider: %v", err)
	}
	if summaryResult == nil || !summaryResult.Triggered {
		t.Fatalf("expected summary to trigger using the stricter plain-text budget")
	}
	if summaryResult.TokenBudget != 45 {
		t.Fatalf("expected token budget 45 from plain-text trigger, got %d", summaryResult.TokenBudget)
	}

	session, err := store.GetSession(ctx, nil, "sess")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.SummarizedCount == 0 {
		t.Fatalf("expected summarized count to advance")
	}
}

func TestBuildContextForProvider_CompactionIgnoresIndependentPlainTextTrigger(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()

	if _, err := store.EnsureSession(ctx, nil, "sess", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		messages := []persistence.ChatMessage{
			{Role: "user", Content: "user message with enough content to exceed a small plain-text budget", CreatedAt: now.Add(time.Duration(i*2) * time.Second)},
			{Role: "assistant", Content: "assistant message with enough content to exceed a small plain-text budget", CreatedAt: now.Add(time.Duration(i*2+1) * time.Second)},
		}
		if err := store.AppendMessages(ctx, nil, "sess", messages, "a", "model"); err != nil {
			t.Fatalf("AppendMessages: %v", err)
		}
	}

	compactor := &stubCompactor{item: llm.CompactionItem{EncryptedContent: "enc"}}
	manager := NewManager(store, compactor, Config{
		Enabled:                      true,
		ReserveBufferTokens:          5,
		MinKeepLastMessages:          2,
		ContextWindowTokens:          200,
		PlainTextContextWindowTokens: 50,
		SummaryModel:                 "stub",
	})

	_, summaryResult, err := manager.BuildContextForProvider(ctx, nil, "sess", compactor, "target-model", SummaryPolicy{
		TargetContextWindowTokens:    200,
		PlainTextContextWindowTokens: 50,
	})
	if err != nil {
		t.Fatalf("BuildContextForProvider: %v", err)
	}
	if summaryResult != nil && summaryResult.Triggered {
		t.Fatalf("expected compaction path to ignore the smaller plain-text trigger until target budget is exceeded")
	}
	if compactor.calls != 0 {
		t.Fatalf("expected no compaction attempt yet, got %d", compactor.calls)
	}
}

func TestManagerBuildContextForcesSummaryWhenTailTooLong(t *testing.T) {
	ctx := context.Background()
	store := newStubChatStore()

	if _, err := store.EnsureSession(ctx, nil, "sess", "Chat"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	now := time.Now().UTC()
	// Keep messages short so we don't hit token-budget summarization.
	for i := 0; i < 6; i++ {
		messages := []persistence.ChatMessage{
			{Role: "user", Content: "u", CreatedAt: now.Add(time.Duration(i*2) * time.Second)},
			{Role: "assistant", Content: "a", CreatedAt: now.Add(time.Duration(i*2+1) * time.Second)},
		}
		if err := store.AppendMessages(ctx, nil, "sess", messages, "a", "model"); err != nil {
			t.Fatalf("AppendMessages: %v", err)
		}
	}

	manager := NewManager(store, &stubLLM{response: "summary"}, Config{
		Enabled:             true,
		ReserveBufferTokens: 1,
		MinKeepLastMessages: 2,
		MaxKeepLastMessages: 4,
		ContextWindowTokens: 50_000,
		SummaryModel:        "stub",
	})

	history, summaryResult, err := manager.BuildContextForProvider(ctx, nil, "sess", nil, "", SummaryPolicy{})
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if summaryResult == nil || !summaryResult.Triggered {
		t.Fatalf("expected summaryResult.Triggered to be true")
	}
	// Expect: 1 system summary + last 4 messages (raw).
	if len(history) != 5 {
		t.Fatalf("expected 5 messages (summary + 4 tail), got %d", len(history))
	}
	if history[0].Role != "system" {
		t.Fatalf("expected system summary message, got %#v", history[0])
	}

	session, err := store.GetSession(ctx, nil, "sess")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.SummarizedCount != 8 {
		// total messages = 12, keep tail 4 => summarizedCount should advance to 8
		t.Fatalf("expected summarized count 8, got %d", session.SummarizedCount)
	}
}
