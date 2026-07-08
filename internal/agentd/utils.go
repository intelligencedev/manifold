package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	"manifold/internal/llm"
	persist "manifold/internal/persistence"
)

// AgentRun represents a single agent invocation for the Runs view (in-memory only).
type AgentRun struct {
	ID        string `json:"id"`
	Prompt    string `json:"prompt"`
	CreatedAt string `json:"createdAt"`
	Status    string `json:"status"` // running | failed | completed
	Tokens    int    `json:"tokens,omitempty"`
}

type runStore struct {
	mu   sync.RWMutex
	runs []AgentRun
}

func newRunStore() *runStore {
	return &runStore{runs: make([]AgentRun, 0, 64)}
}

func (s *runStore) create(prompt string) AgentRun {
	return s.createWithID(fmt.Sprintf("run_%d", time.Now().UnixNano()), prompt, time.Now().UTC())
}

func (s *runStore) createWithID(id string, prompt string, createdAt time.Time) AgentRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := AgentRun{
		ID:        id,
		Prompt:    prompt,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
		Status:    "running",
	}
	s.runs = append(s.runs, run)
	return run
}

func (s *runStore) updateStatus(id string, status string, tokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.runs {
		if s.runs[i].ID == id {
			s.runs[i].Status = status
			if tokens > 0 {
				s.runs[i].Tokens = tokens
			}
			break
		}
	}
}

func (s *runStore) list() []AgentRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AgentRun, len(s.runs))
	copy(out, s.runs)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func (s *runStore) get(id string) (AgentRun, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, run := range s.runs {
		if run.ID == id {
			return run, true
		}
	}
	return AgentRun{}, false
}

// withMaybeTimeout returns a context derived from parent with an optional timeout.
func withMaybeTimeout(parent context.Context, seconds int) (context.Context, context.CancelFunc, time.Duration) {
	if seconds > 0 {
		d := time.Duration(seconds) * time.Second
		ctx, cancel := context.WithTimeout(parent, d)
		return ctx, cancel, d
	}
	ctx, cancel := context.WithCancel(parent)
	return ctx, cancel, 0
}

func ensureChatSession(ctx context.Context, store persist.ChatStore, userID *int64, sessionID string) (persist.ChatSession, error) {
	return store.EnsureSession(ctx, userID, sessionID, "Conversation")
}

func cleanupEphemeralChatSession(store persist.ChatStore, userID *int64, sessionID string) {
	if store == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.DeleteSession(ctx, userID, sessionID); err != nil && !errors.Is(err, persist.ErrNotFound) {
		log.Warn().Err(err).Str("session", sessionID).Msg("cleanup_ephemeral_chat_session_failed")
	}
}

func previewSnippet(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	collapsed := strings.Join(strings.Fields(content), " ")
	runes := []rune(collapsed)
	if len(runes) <= 80 {
		return collapsed
	}
	limit := min(77, len(runes))
	return string(runes[:limit]) + "..."
}

type chatTurnRecord struct {
	UserID           *int64
	SessionID        string
	UserContent      string
	AssistantContent string
	DurationMs       *int64
	Model            string
}

func storeChatTurn(ctx context.Context, store persist.ChatStore, record chatTurnRecord) error {
	messages := make([]persist.ChatMessage, 0, 2)
	now := time.Now().UTC()
	if strings.TrimSpace(record.UserContent) != "" {
		messages = append(messages, persist.ChatMessage{
			SessionID: record.SessionID,
			Role:      "user",
			Content:   record.UserContent,
			CreatedAt: now,
		})
	}
	if strings.TrimSpace(record.AssistantContent) != "" {
		messages = append(messages, persist.ChatMessage{
			SessionID:  record.SessionID,
			Role:       "assistant",
			Content:    record.AssistantContent,
			CreatedAt:  now.Add(2 * time.Millisecond),
			DurationMs: record.DurationMs,
		})
	}
	if len(messages) == 0 {
		return nil
	}
	preview := previewSnippet(record.AssistantContent)
	if preview == "" {
		preview = previewSnippet(record.UserContent)
	}
	return store.AppendMessages(ctx, record.UserID, record.SessionID, messages, preview, record.Model)
}

// storeChatTurnWithHistory stores a complete conversation turn including all intermediate
// assistant messages (with tool calls) and tool response messages.
type chatTurnHistoryRecord struct {
	UserID             *int64
	SessionID          string
	UserMessageID      string
	UserContent        string
	TurnMessages       []llm.Message
	FinalContent       string
	AssistantMessageID string
	DurationMs         *int64
	Model              string
}

const chatTurnMessageSpacing = time.Microsecond

func storeChatTurnWithHistory(ctx context.Context, store persist.ChatStore, record chatTurnHistoryRecord) error {
	roles := make([]string, len(record.TurnMessages))
	lastAssistantIndex := -1
	for i, m := range record.TurnMessages {
		roles[i] = m.Role
		if m.Role == "assistant" {
			lastAssistantIndex = i
		}
	}
	log.Info().Str("session_id", record.SessionID).Str("user_content_len", fmt.Sprint(len(record.UserContent))).Int("turn_messages", len(record.TurnMessages)).Strs("roles", roles).Msg("store_chat_turn_start")
	messages := make([]persist.ChatMessage, 0, 2+len(record.TurnMessages))
	now := existingUserMessageCreatedAt(ctx, store, record)
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Add user message
	if strings.TrimSpace(record.UserContent) != "" {
		messages = append(messages, persist.ChatMessage{
			ID:        strings.TrimSpace(record.UserMessageID),
			SessionID: record.SessionID,
			Role:      "user",
			Content:   record.UserContent,
			CreatedAt: now,
		})
	}

	// Add all intermediate turn messages (assistant with tool calls, tool responses)
	for i, msg := range record.TurnMessages {
		// Serialize the message to preserve tool calls and tool IDs
		var content string
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// Serialize assistant messages with tool calls as JSON
			b, err := json.Marshal(map[string]any{
				"content":    msg.Content,
				"tool_calls": msg.ToolCalls,
			})
			if err == nil {
				content = string(b)
			} else {
				content = msg.Content
			}
		} else if msg.Role == "tool" {
			// Serialize tool messages with tool_id
			b, err := json.Marshal(map[string]any{
				"content": msg.Content,
				"tool_id": msg.ToolID,
			})
			if err == nil {
				content = string(b)
			} else {
				content = msg.Content
			}
		} else {
			content = msg.Content
		}

		messageID := ""
		if msg.Role == "assistant" && i == lastAssistantIndex {
			messageID = strings.TrimSpace(record.AssistantMessageID)
		} else if strings.TrimSpace(record.AssistantMessageID) != "" || strings.TrimSpace(record.UserMessageID) != "" {
			messageID = stableTurnMessageID(record, msg.Role, i)
		}
		messages = append(messages, persist.ChatMessage{
			ID:         messageID,
			SessionID:  record.SessionID,
			Role:       msg.Role,
			Content:    content,
			CreatedAt:  now.Add(time.Duration(i+1) * chatTurnMessageSpacing),
			DurationMs: durationForTurnMessage(record, msg.Role, i, lastAssistantIndex),
		})
	}

	if len(messages) == 0 {
		return nil
	}

	preview := previewSnippet(record.FinalContent)
	if preview == "" && len(record.TurnMessages) > 0 {
		preview = previewSnippet(record.TurnMessages[len(record.TurnMessages)-1].Content)
	}
	if preview == "" {
		preview = previewSnippet(record.UserContent)
	}
	return store.AppendMessagesOnce(ctx, record.UserID, record.SessionID, messages, preview, record.Model)
}

func existingUserMessageCreatedAt(ctx context.Context, store persist.ChatStore, record chatTurnHistoryRecord) time.Time {
	if store == nil || strings.TrimSpace(record.SessionID) == "" || strings.TrimSpace(record.UserMessageID) == "" {
		return time.Time{}
	}
	messages, err := store.ListMessages(ctx, record.UserID, record.SessionID, 0)
	if err != nil {
		return time.Time{}
	}
	for _, message := range messages {
		if message.ID == record.UserMessageID && message.Role == "user" && !message.CreatedAt.IsZero() {
			return message.CreatedAt.UTC()
		}
	}
	return time.Time{}
}

func durationForTurnMessage(record chatTurnHistoryRecord, role string, index, lastAssistantIndex int) *int64 {
	if role != "assistant" || index != lastAssistantIndex {
		return nil
	}
	return record.DurationMs
}

func stableTurnMessageID(record chatTurnHistoryRecord, role string, index int) string {
	base := strings.TrimSpace(record.AssistantMessageID)
	if base == "" {
		base = strings.TrimSpace(record.UserMessageID)
	}
	if base == "" {
		return ""
	}
	seed := fmt.Sprintf("chat:%s:%s:%s:%d", record.SessionID, base, role, index)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String()
}

func resolveChatAccess(_ context.Context, authStore *auth.Store, user *auth.User) (*int64, bool, error) {
	if authStore == nil || user == nil {
		return nil, true, nil
	}
	id := user.ID
	return &id, false, nil
}

func setChatCORSHeaders(w http.ResponseWriter, r *http.Request, methods string) {
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
	if methods != "" {
		w.Header().Set("Access-Control-Allow-Methods", methods)
	}
}

func (a *app) requireUserID(r *http.Request) (int64, error) {
	if !a.cfg.Auth.Enabled {
		return systemUserID, nil
	}
	user, ok := auth.CurrentUser(r.Context())
	if !ok || user == nil {
		return 0, errors.New("unauthorized")
	}
	return user.ID, nil
}

// providerSupportsCompaction checks if an LLM provider implements the CompactionProvider
// interface, which is required for using OpenAI Responses API compaction summaries.
// Non-OpenAI providers (Anthropic, Google, etc.) do not support compaction.
func providerSupportsCompaction(provider llm.Provider) bool {
	return llm.ProviderSupportsCompaction(provider)
}

func summaryEndpointSupportsResponsesCompaction(providerName, api, summaryBaseURL string) bool {
	providerName = strings.TrimSpace(providerName)
	if providerName != "" && !strings.EqualFold(providerName, "openai") {
		return false
	}
	if !strings.EqualFold(api, "responses") {
		return false
	}
	return isOpenAIResponsesCompactionBaseURL(summaryBaseURL)
}

func isOpenAIResponsesCompactionBaseURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "api.openai.com"
}
