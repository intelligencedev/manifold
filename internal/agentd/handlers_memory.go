package agentd

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/agent/memory"
	"manifold/internal/auth"
	"manifold/internal/llm"
)

// memoryPlanResponse exposes how chat memory is currently sizing history for a session.
// It mirrors the heuristics in internal/agent/memory.Manager and Engine.maybeSummarize.
type memoryPlanResponse struct {
	ContextWindowTokens    int `json:"contextWindowTokens"`
	ReserveBufferTokens    int `json:"reserveBufferTokens"`
	TokenBudget            int `json:"tokenBudget"`
	TailTokenBudget        int `json:"tailTokenBudget"`
	MinKeepLastMessages    int `json:"minKeepLastMessages"`
	MaxSummaryChunkTokens  int `json:"maxSummaryChunkTokens"`
	EstimatedHistoryTokens int `json:"estimatedHistoryTokens"`
	EstimatedTailTokens    int `json:"estimatedTailTokens"`
	TailStartIndex         int `json:"tailStartIndex"`
	TotalMessages          int `json:"totalMessages"`
}

// debugMemorySessionResponse contains high-level memory state for a chat session.
type debugMemorySessionResponse struct {
	Session         any                `json:"session"`
	Summary         string             `json:"summary"`
	SummarizedCount int                `json:"summarizedCount"`
	Messages        []llm.Message      `json:"messages"`
	Plan            memoryPlanResponse `json:"plan"`
}

// debugMemoryEvolvingResponse exposes evolving/ReMem state for observability.
type debugMemoryEvolvingResponse struct {
	Enabled      bool                       `json:"enabled"`
	TotalEntries int                        `json:"totalEntries"`
	TopK         int                        `json:"topK"`
	MaxSize      int                        `json:"maxSize"`
	WindowSize   int                        `json:"windowSize"`
	RecentWindow []*memory.MemoryEntry      `json:"recentWindow"`
	LastQuery    string                     `json:"lastQuery,omitempty"`
	Retrieved    []memory.ScoredMemoryEntry `json:"retrieved,omitempty"`
}

// debugMemoryHandler serves read-only observability for chat and evolving memory.
// All endpoints are nested under /debug/memory and require authentication when
// auth is enabled.
func (a *app) debugMemoryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Auth gate if enabled
		if a.cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Normalize the path so that both /debug/memory and /api/debug/memory
		// prefixes are supported. The router already wires both prefixes to this
		// handler, but without this normalization a request like
		//   /api/debug/memory/evolving
		// would not match the /debug/memory prefix below, leading to a 404 even
		// though the route is correctly registered.
		basePath := "/debug/memory"
		if strings.HasPrefix(r.URL.Path, "/api/debug/memory") {
			basePath = "/api/debug/memory"
		}

		path := strings.TrimPrefix(r.URL.Path, basePath)
		path = strings.Trim(path, "/")
		if path == "" {
			writeJSON(w, http.StatusOK, map[string]string{
				"sessions": "/debug/memory/sessions",
				"entries":  "/debug/memory/entries",
				"plan":     "/debug/memory/plan",
				"evolving": "/debug/memory/evolving",
			})
			return
		}

		switch {
		case path == "sessions":
			// High-level inventory of chat sessions (id, name, summary length, etc.)
			a.handleDebugMemorySessions(w, r)
		case strings.HasPrefix(path, "sessions/"):
			// Per-session: summary + tail as seen by BuildContext
			a.handleDebugMemorySessionDetail(w, r, strings.TrimPrefix(path, "sessions/"))
		case path == "entries":
			// Flat list of chat messages across a session (with filtering/pagination)
			a.handleDebugMemoryEntries(w, r)
		case path == "plan":
			// Derived memory plan for a session
			a.handleDebugMemoryPlan(w, r)
		case path == "evolving":
			// Evolving memory / ReMem introspection
			a.handleDebugMemoryEvolving(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func (a *app) handleDebugMemorySessions(w http.ResponseWriter, r *http.Request) {
	var userID *int64
	if a.cfg.Auth.Enabled {
		uid, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID = &uid
	}
	sessions, err := a.chatStore.ListSessions(r.Context(), userID)
	if err != nil {
		log.Error().Err(err).Msg("debug_memory_list_sessions")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (a *app) handleDebugMemorySessionDetail(w http.ResponseWriter, r *http.Request, sessionID string) {
	var userID *int64
	if a.cfg.Auth.Enabled {
		uid, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID = &uid
	}

	sess, err := a.chatStore.GetSession(r.Context(), userID, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("debug_memory_get_session")
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Rebuild the LLM context for this session using the same logic as /agent/run
	ctxMsgs, _, err := a.chatMemory.BuildContext(r.Context(), userID, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("debug_memory_build_context")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Derive a lightweight plan for this session using token-aware heuristics.
	plan := a.deriveMemoryPlan(sess, ctxMsgs)

	resp := debugMemorySessionResponse{
		Session:         sess,
		Summary:         sess.Summary,
		SummarizedCount: sess.SummarizedCount,
		Messages:        ctxMsgs,
		Plan:            plan,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *app) handleDebugMemoryEntries(w http.ResponseWriter, r *http.Request) {
	var userID *int64
	if a.cfg.Auth.Enabled {
		uid, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID = &uid
	}

	q := r.URL.Query()
	sessionID := strings.TrimSpace(q.Get("session_id"))
	limitStr := strings.TrimSpace(q.Get("limit"))
	limit := 0
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}

	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	msgs, err := a.chatStore.ListMessages(r.Context(), userID, sessionID, limit)
	if err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("debug_memory_list_messages")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (a *app) handleDebugMemoryPlan(w http.ResponseWriter, r *http.Request) {
	var userID *int64
	if a.cfg.Auth.Enabled {
		uid, err := a.requireUserID(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID = &uid
	}

	q := r.URL.Query()
	sessionID := strings.TrimSpace(q.Get("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	sess, err := a.chatStore.GetSession(r.Context(), userID, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("debug_memory_get_session")
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	ctxMsgs, _, err := a.chatMemory.BuildContext(r.Context(), userID, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session", sessionID).Msg("debug_memory_build_context")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	plan := a.deriveMemoryPlan(sess, ctxMsgs)
	writeJSON(w, http.StatusOK, plan)
}

func (a *app) deriveMemoryPlan(sess any, msgs []llm.Message) memoryPlanResponse {
	// We approximate the tail as everything except the first summary/system message
	total := len(msgs)
	tailStart := 0
	estTotal := 0
	for _, m := range msgs {
		estTotal += len([]rune(strings.TrimSpace(m.Content)))/4 + 1
	}
	if total > 0 && msgs[0].Role == "system" && strings.Contains(msgs[0].Content, "Conversation summary") {
		tailStart = 1
	}
	estTail := 0
	for _, m := range msgs[tailStart:] {
		estTail += len([]rune(strings.TrimSpace(m.Content)))/4 + 1
	}

	ctxTokens := a.chatMemory.ContextWindowTokens()
	if ctxTokens <= 0 {
		ctxTokens = 128_000
	}
	reserveBuffer := a.chatMemory.ReserveBufferTokens()
	if reserveBuffer <= 0 {
		reserveBuffer = 25_000
	}
	tokenBudget := ctxTokens - reserveBuffer
	if tokenBudget <= 0 {
		tokenBudget = ctxTokens / 2
	}
	tailBudget := tokenBudget / 2
	if tailBudget <= 0 {
		tailBudget = tokenBudget
	}

	return memoryPlanResponse{
		ContextWindowTokens:    ctxTokens,
		ReserveBufferTokens:    reserveBuffer,
		TokenBudget:            tokenBudget,
		TailTokenBudget:        tailBudget,
		MinKeepLastMessages:    a.chatMemory.MinKeepLastMessages(),
		MaxSummaryChunkTokens:  a.chatMemory.MaxSummaryChunkTokens(),
		EstimatedHistoryTokens: estTotal,
		EstimatedTailTokens:    estTail,
		TailStartIndex:         tailStart,
		TotalMessages:          total,
	}
}

func (a *app) handleDebugMemoryEvolving(w http.ResponseWriter, r *http.Request) {
	eng := a.engine
	if eng == nil || eng.EvolvingMemory == nil {
		writeJSON(w, http.StatusOK, debugMemoryEvolvingResponse{Enabled: false})
		return
	}

	// Evolving memory is currently only attached to the shared engine instance.
	// When auth is enabled, do not expose system-level evolving memory to
	// non-admin users.
	if a.cfg.Auth.Enabled {
		u, ok := auth.CurrentUser(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !auth.HasRole(u, "admin") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	// Base snapshot
	entries := eng.EvolvingMemory.ExportMemories()
	resp := debugMemoryEvolvingResponse{
		Enabled:      true,
		TotalEntries: len(entries),
		TopK:         eng.EvolvingMemory.TopK(),
		MaxSize:      eng.EvolvingMemory.MaxSize(),
		WindowSize:   eng.EvolvingMemory.WindowSize(),
		RecentWindow: eng.EvolvingMemory.GetRecentWindow(),
	}

	q := strings.TrimSpace(r.URL.Query().Get("query"))
	if q != "" {
		resp.LastQuery = q
		if scored, err := eng.EvolvingMemory.SearchWithScores(r.Context(), q); err == nil {
			resp.Retrieved = scored
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
