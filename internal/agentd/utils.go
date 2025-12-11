package agentd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	"manifold/internal/config"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	run := AgentRun{
		ID:        fmt.Sprintf("run_%d", time.Now().UnixNano()),
		Prompt:    prompt,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
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

func previewSnippet(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	collapsed := strings.Join(strings.Fields(content), " ")
	runes := []rune(collapsed)
	if len(runes) <= 80 {
		return collapsed
	}
	limit := 77
	if limit > len(runes) {
		limit = len(runes)
	}
	return string(runes[:limit]) + "..."
}

func storeChatTurn(ctx context.Context, store persist.ChatStore, userID *int64, sessionID, userContent, assistantContent, model string) error {
	messages := make([]persist.ChatMessage, 0, 2)
	now := time.Now().UTC()
	if strings.TrimSpace(userContent) != "" {
		messages = append(messages, persist.ChatMessage{
			SessionID: sessionID,
			Role:      "user",
			Content:   userContent,
			CreatedAt: now,
		})
	}
	if strings.TrimSpace(assistantContent) != "" {
		messages = append(messages, persist.ChatMessage{
			SessionID: sessionID,
			Role:      "assistant",
			Content:   assistantContent,
			CreatedAt: now.Add(2 * time.Millisecond),
		})
	}
	if len(messages) == 0 {
		return nil
	}
	preview := previewSnippet(assistantContent)
	if preview == "" {
		preview = previewSnippet(userContent)
	}
	return store.AppendMessages(ctx, userID, sessionID, messages, preview, model)
}

// storeChatTurnWithHistory stores a complete conversation turn including all intermediate
// assistant messages (with tool calls) and tool response messages.
func storeChatTurnWithHistory(ctx context.Context, store persist.ChatStore, userID *int64, sessionID, userContent string, turnMessages []llm.Message, finalContent, model string) error {
	roles := make([]string, len(turnMessages))
	for i, m := range turnMessages {
		roles[i] = m.Role
	}
	log.Info().Str("session_id", sessionID).Str("user_content_len", fmt.Sprint(len(userContent))).Int("turn_messages", len(turnMessages)).Strs("roles", roles).Msg("store_chat_turn_start")
	messages := make([]persist.ChatMessage, 0, 2+len(turnMessages))
	now := time.Now().UTC()

	// Add user message
	if strings.TrimSpace(userContent) != "" {
		messages = append(messages, persist.ChatMessage{
			SessionID: sessionID,
			Role:      "user",
			Content:   userContent,
			CreatedAt: now,
		})
	}

	// Add all intermediate turn messages (assistant with tool calls, tool responses)
	for i, msg := range turnMessages {
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

		messages = append(messages, persist.ChatMessage{
			SessionID: sessionID,
			Role:      msg.Role,
			Content:   content,
			CreatedAt: now.Add(time.Duration(i+1) * 10 * time.Millisecond),
		})
	}

	if len(messages) == 0 {
		return nil
	}

	preview := previewSnippet(finalContent)
	if preview == "" && len(turnMessages) > 0 {
		preview = previewSnippet(turnMessages[len(turnMessages)-1].Content)
	}
	if preview == "" {
		preview = previewSnippet(userContent)
	}
	return store.AppendMessages(ctx, userID, sessionID, messages, preview, model)
}

func resolveChatAccess(ctx context.Context, authStore *auth.Store, user *auth.User) (*int64, bool, error) {
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

func databasesTestPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(cctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func specialistsFromStore(list []persist.Specialist) []config.SpecialistConfig {
	out := make([]config.SpecialistConfig, 0, len(list))
	for _, s := range list {
		if strings.EqualFold(strings.TrimSpace(s.Name), "orchestrator") {
			continue
		}
		out = append(out, config.SpecialistConfig{
			Name:            s.Name,
			Description:     s.Description,
			Provider:        s.Provider,
			BaseURL:         s.BaseURL,
			APIKey:          s.APIKey,
			Model:           s.Model,
			EnableTools:     s.EnableTools,
			Paused:          s.Paused,
			AllowTools:      s.AllowTools,
			ReasoningEffort: s.ReasoningEffort,
			System:          s.System,
			ExtraHeaders:    s.ExtraHeaders,
			ExtraParams:     s.ExtraParams,
		})
	}
	return out
}

// wavFloat32 converts a little-endian uint32 representation to float32 without allocations.
func wavFloat32(bits uint32) float32 {
	return *(*float32)(unsafe.Pointer(&bits))
}
