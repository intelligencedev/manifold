package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/rs/zerolog/log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"manifold/internal/agent"
	"manifold/internal/agent/memory"
	"manifold/internal/agent/prompts"
	"manifold/internal/auth"
	"manifold/internal/config"
	"manifold/internal/httpapi"
	llmpkg "manifold/internal/llm"
	openaillm "manifold/internal/llm/openai"
	"manifold/internal/mcpclient"
	"manifold/internal/observability"
	persist "manifold/internal/persistence"
	"manifold/internal/persistence/databases"
	persistdb "manifold/internal/persistence/databases"
	"manifold/internal/playground"
	"manifold/internal/playground/artifacts"
	"manifold/internal/playground/dataset"
	"manifold/internal/playground/eval"
	"manifold/internal/playground/experiment"
	"manifold/internal/playground/provider"
	playgroundregistry "manifold/internal/playground/registry"
	"manifold/internal/playground/worker"
	"manifold/internal/specialists"
	"manifold/internal/tools"
	"manifold/internal/tools/cli"
	"manifold/internal/tools/db"
	"manifold/internal/tools/imagetool"
	llmtools "manifold/internal/tools/llmtool"
	"manifold/internal/tools/patchtool"
	specialists_tool "manifold/internal/tools/specialists"
	"manifold/internal/tools/tts"
	"manifold/internal/tools/utility"
	"manifold/internal/tools/web"
	"manifold/internal/warpp"
	"manifold/internal/webui"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// AgentRun represents a single agent invocation for the Runs view (in-memory only)
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
	// Return newest-first for convenience
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// withMaybeTimeout returns a context derived from parent with an optional timeout.
// If seconds <= 0, no timeout is applied and the returned duration is 0.
func withMaybeTimeout(parent context.Context, seconds int) (context.Context, context.CancelFunc, time.Duration) {
	if seconds > 0 {
		d := time.Duration(seconds) * time.Second
		ctx, cancel := context.WithTimeout(parent, d)
		return ctx, cancel, d
	}
	ctx, cancel := context.WithCancel(parent)
	return ctx, cancel, 0
}

func ensureChatSession(ctx context.Context, store persist.ChatStore, sessionID string) (persist.ChatSession, error) {
	return store.EnsureSession(ctx, sessionID, "Conversation")
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

func storeChatTurn(ctx context.Context, store persist.ChatStore, sessionID, userContent, assistantContent, model string) error {
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
	return store.AppendMessages(ctx, sessionID, messages, preview, model)
}

func main() {
	// Load environment from .env (or fallback to example.env) so local
	// development can run without exporting variables manually. Do this
	// before initializing the logger so LOG_PATH/LOG_LEVEL are respected.
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load("example.env")
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		log.Fatal().Err(err).Msg("failed to load config")
	}

	// Initialize logger next (after .env has been loaded)
	observability.InitLogger(cfg.LogPath, cfg.LogLevel)

	shutdown, err := observability.InitOTel(context.Background(), cfg.Obs)
	if err != nil {
		// don't abort startup for observability failures; log and continue
		log.Warn().Err(err).Msg("otel init failed, continuing without observability")
		shutdown = nil
	}
	if shutdown != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	httpClient := observability.NewHTTPClient(nil)
	if len(cfg.OpenAI.ExtraHeaders) > 0 {
		httpClient = observability.WithHeaders(httpClient, cfg.OpenAI.ExtraHeaders)
	}
	llmpkg.ConfigureLogging(cfg.LogPayloads, cfg.OutputTruncateByte)
	llm := openaillm.New(cfg.OpenAI, httpClient)
	summaryCfg := cfg.OpenAI
	summaryCfg.Model = cfg.OpenAI.SummaryModel
	summaryCfg.BaseURL = cfg.OpenAI.SummaryBaseURL
	summaryLLM := openaillm.New(summaryCfg, httpClient)

	toolRegistry := tools.NewRegistryWithLogging(cfg.LogPayloads)
	// Keep a handle to the full, unfiltered registry so we can toggle
	// orchestrator tool exposure at runtime when edited via the UI.
	baseToolRegistry := toolRegistry
	// Databases: construct backends and register tools
	mgr, err := databases.NewManager(context.Background(), cfg.Databases)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init databases")
	}
	exec := cli.NewExecutor(cfg.Exec, cfg.Workdir, cfg.OutputTruncateByte)
	toolRegistry.Register(cli.NewTool(exec))
	toolRegistry.Register(web.NewTool(cfg.Web.SearXNGURL))
	toolRegistry.Register(web.NewFetchTool())
	toolRegistry.Register(patchtool.New(cfg.Workdir))
	toolRegistry.Register(utility.NewTextboxTool())
	toolRegistry.Register(tts.New(cfg, httpClient))
	toolRegistry.Register(db.NewSearchIndexTool(mgr.Search))
	toolRegistry.Register(db.NewSearchQueryTool(mgr.Search))
	toolRegistry.Register(db.NewSearchRemoveTool(mgr.Search))
	toolRegistry.Register(db.NewVectorUpsertTool(mgr.Vector, cfg.Embedding))
	toolRegistry.Register(db.NewVectorQueryTool(mgr.Vector))
	toolRegistry.Register(db.NewVectorDeleteTool(mgr.Vector))
	toolRegistry.Register(db.NewGraphUpsertNodeTool(mgr.Graph))
	toolRegistry.Register(db.NewGraphUpsertEdgeTool(mgr.Graph))
	toolRegistry.Register(db.NewGraphNeighborsTool(mgr.Graph))
	toolRegistry.Register(db.NewGraphGetNodeTool(mgr.Graph))
	// Provider factory for base_url override in llm_transform
	newProv := func(baseURL string) llmpkg.Provider {
		c2 := cfg.OpenAI
		c2.BaseURL = baseURL
		return openaillm.New(c2, httpClient)
	}
	toolRegistry.Register(llmtools.NewTransform(llm, cfg.OpenAI.Model, newProv))
	toolRegistry.Register(imagetool.NewDescribeTool(llm, cfg.Workdir, cfg.OpenAI.Model, newProv))
	// Specialists tool for LLM-driven routing
	specReg := specialists.NewRegistry(cfg.OpenAI, cfg.Specialists, httpClient, toolRegistry)
	toolRegistry.Register(specialists_tool.New(specReg))

	// If tools are globally disabled, use an empty registry. Otherwise, apply a
	// top-level allow list only to the main orchestrator by wrapping the base registry.
	if !cfg.EnableTools {
		toolRegistry = tools.NewRegistry() // Empty registry for orchestrator-only
	} else if len(cfg.ToolAllowList) > 0 {
		toolRegistry = tools.NewFilteredRegistry(baseToolRegistry, cfg.ToolAllowList)
	}

	// Debug: log which tools are exposed after any filtering so we can diagnose
	// missing tool registrations at runtime.
	{
		names := make([]string, 0, len(toolRegistry.Schemas()))
		for _, s := range toolRegistry.Schemas() {
			names = append(names, s.Name)
		}
		log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents")
	}

	// MCP: connect to configured servers and register their tools
	mcpMgr := mcpclient.NewManager()
	ctxInit, cancelInit := context.WithTimeout(context.Background(), 20*time.Second)
	_ = mcpMgr.RegisterFromConfig(ctxInit, toolRegistry, cfg.MCP)
	cancelInit()

	// WARPP runner with Postgres-backed persistence (no filesystem writes)
	workflowDir := "configs/workflows" // read-only seed directory
	var warppMu sync.RWMutex
	var wfreg *warpp.Registry
	var wfStore persist.WarppWorkflowStore
	{
		// Initialize store: Postgres if configured, else in-memory fallback.
		if cfg.Databases.DefaultDSN != "" {
			if p, errPool := databasesTestPool(context.Background(), cfg.Databases.DefaultDSN); errPool == nil {
				wfStore = persistdb.NewPostgresWarppStore(p)
			}
		}
		if wfStore == nil {
			wfStore = persistdb.NewPostgresWarppStore(nil)
		}
		_ = wfStore.Init(context.Background())

		// Load from store; seed from defaults if empty
		if list, err := wfStore.ListWorkflows(context.Background()); err == nil && len(list) > 0 {
			wfreg = &warpp.Registry{}
			for _, pw := range list {
				b, _ := json.Marshal(pw)
				var w warpp.Workflow
				if err := json.Unmarshal(b, &w); err == nil {
					wfreg.Upsert(w, "")
				}
			}
		} else {
			wfreg, _ = warpp.LoadFromDir(workflowDir)
			for _, w := range wfreg.All() {
				b, _ := json.Marshal(w)
				var pw persist.WarppWorkflow
				if err := json.Unmarshal(b, &pw); err == nil {
					_, _ = wfStore.Upsert(context.Background(), pw)
				}
			}
		}
	}
	warppRunner := &warpp.Runner{Workflows: wfreg, Tools: toolRegistry}

	eng := &agent.Engine{
		LLM:              llm,
		Tools:            toolRegistry,
		MaxSteps:         cfg.MaxSteps,
		System:           prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt),
		Model:            cfg.OpenAI.Model,
		SummaryEnabled:   cfg.SummaryEnabled,
		SummaryThreshold: cfg.SummaryThreshold,
		SummaryKeepLast:  cfg.SummaryKeepLast,
	}

	// Chat history store (Postgres-backed when configured)
	chatStore := mgr.Chat
	if chatStore == nil {
		log.Fatal().Msg("chat store not initialized")
	}
	chatMemory := memory.NewManager(chatStore, summaryLLM, memory.Config{
		Enabled:      cfg.SummaryEnabled,
		Threshold:    cfg.SummaryThreshold,
		KeepLast:     cfg.SummaryKeepLast,
		SummaryModel: cfg.OpenAI.SummaryModel,
	})

	if mgr.Playground == nil {
		log.Fatal().Msg("playground store not initialized; set databases.defaultDSN or chat DSN")
	}
	artifactDir := filepath.Join(cfg.Workdir, "playground-artifacts")
	artifactStore := artifacts.NewFilesystemStore(artifactDir)
	playgroundRegistry := playgroundregistry.New(mgr.Playground)
	playgroundDataset := dataset.NewService(mgr.Playground)
	playgroundRepo := experiment.NewRepository()
	playgroundPlanner := experiment.NewPlanner(experiment.PlannerConfig{MaxRowsPerShard: 32, MaxVariantsPerShard: 4})
	playgroundProvider := provider.NewLLMAdapter(llm, cfg.OpenAI.Model)
	playgroundWorker := worker.NewWorker(playgroundProvider, artifactStore)
	playgroundEvals := eval.NewRunner(eval.NewRegistry(), playgroundProvider)
	playgroundService := playground.NewService(playground.Config{MaxConcurrentShards: 4}, playgroundRegistry, playgroundDataset, playgroundRepo, playgroundPlanner, playgroundWorker, playgroundEvals, mgr.Playground)
	playgroundAPI := httpapi.NewServer(playgroundService)

	// Initialize in-memory run store for Runs view
	runs := newRunStore()

	// Initialize Whisper model for speech-to-text (optional – if model file present)
	var whisperModel whisper.Model
	modelPath := "models/ggml-small.en.bin"
	if model, err := whisper.New(modelPath); err == nil {
		whisperModel = model
		log.Info().Str("model", modelPath).Msg("whisper model loaded")
	} else {
		log.Warn().Str("model", modelPath).Err(err).Msg("whisper model load failed; /stt disabled")
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/playground", playgroundAPI)
	mux.Handle("/api/v1/playground/", playgroundAPI)
	// AUTH setup
	var authStore *auth.Store
	var oidcAuth *auth.OIDC
	if cfg.Auth.Enabled {
		// Reuse default DB DSN for sessions/users if available
		dsn := cfg.Databases.DefaultDSN
		if dsn == "" {
			log.Fatal().Msg("auth enabled but databases.defaultDSN is empty")
		}
		pool, err := databasesTestPool(context.Background(), dsn)
		if err != nil {
			log.Fatal().Err(err).Msg("auth db connect failed")
		}
		authStore = auth.NewStore(pool, cfg.Auth.SessionTTLHours)
		if err := authStore.InitSchema(context.Background()); err != nil {
			log.Fatal().Err(err).Msg("auth schema init failed")
		}
		_ = authStore.EnsureDefaultRoles(context.Background())
		// OIDC provider
		issuer := cfg.Auth.IssuerURL
		clientID := cfg.Auth.ClientID
		clientSecret := cfg.Auth.ClientSecret
		redirectURL := cfg.Auth.RedirectURL
		var errOIDC error
		oidcAuth, errOIDC = auth.NewOIDC(context.Background(), issuer, clientID, clientSecret, redirectURL, authStore, cfg.Auth.CookieName, cfg.Auth.AllowedDomains, cfg.Auth.StateTTLSeconds, cfg.Auth.CookieSecure)
		if errOIDC != nil {
			log.Fatal().Err(errOIDC).Msg("oidc init failed")
		}
		// public auth endpoints
		mux.HandleFunc("/auth/login", oidcAuth.LoginHandler())
		mux.HandleFunc("/auth/callback", oidcAuth.CallbackHandler(cfg.Auth.CookieSecure, cfg.Auth.CookieDomain))
		// Wrap logout handler to allow CORS credentialed requests from the web UI
		logoutHandler := oidcAuth.LogoutHandler(cfg.Auth.CookieSecure, cfg.Auth.CookieDomain)
		mux.HandleFunc("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			logoutHandler(w, r)
		})
		mux.HandleFunc("/api/me", oidcAuth.MeHandler())
	}

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ready")
	})

	// Runs list endpoint for the web UI
	// Protected API: runs list
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			// require auth for this endpoint
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runs.list())
	})

	mux.HandleFunc("/api/chat/sessions", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.Method {
		case http.MethodGet:
			sessions, err := chatStore.ListSessions(r.Context())
			if err != nil {
				log.Error().Err(err).Msg("list_chat_sessions")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(sessions); err != nil {
				log.Error().Err(err).Msg("encode_chat_sessions")
			}
		case http.MethodPost:
			defer r.Body.Close()
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			sess, err := chatStore.CreateSession(r.Context(), body.Name)
			if err != nil {
				log.Error().Err(err).Msg("create_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(sess); err != nil {
				log.Error().Err(err).Msg("encode_chat_session")
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/chat/sessions/", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		rest := strings.TrimPrefix(r.URL.Path, "/api/chat/sessions/")
		rest = strings.Trim(rest, "/")
		if rest == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(rest, "/")
		id := parts[0]
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PATCH, DELETE, OPTIONS")
		if len(parts) == 2 && parts[1] == "messages" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if len(parts) == 2 && parts[1] == "messages" {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			limit := 0
			if raw := r.URL.Query().Get("limit"); raw != "" {
				if v, err := strconv.Atoi(raw); err == nil && v > 0 {
					limit = v
				}
			}
			msgs, err := chatStore.ListMessages(r.Context(), id, limit)
			if err != nil {
				log.Error().Err(err).Str("session", id).Msg("list_chat_messages")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(msgs); err != nil {
				log.Error().Err(err).Msg("encode_chat_messages")
			}
			return
		}
		switch r.Method {
		case http.MethodGet:
			sess, ok, err := chatStore.GetSession(r.Context(), id)
			if err != nil {
				log.Error().Err(err).Str("session", id).Msg("get_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(sess); err != nil {
				log.Error().Err(err).Msg("encode_chat_session")
			}
		case http.MethodPatch:
			defer r.Body.Close()
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			sess, err := chatStore.RenameSession(r.Context(), id, body.Name)
			if err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("rename_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(sess); err != nil {
				log.Error().Err(err).Msg("encode_chat_session")
			}
		case http.MethodDelete:
			if err := chatStore.DeleteSession(r.Context(), id); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "not found") {
					http.NotFound(w, r)
					return
				}
				log.Error().Err(err).Str("session", id).Msg("delete_chat_session")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Users & Roles management (admin)
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Auth.Enabled || authStore == nil {
			http.NotFound(w, r)
			return
		}
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		// CORS basics
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.Method {
		case http.MethodGet:
			// Admin only
			if u, ok := auth.CurrentUser(r.Context()); ok {
				okRole, _ := authStore.HasRole(r.Context(), u.ID, "admin")
				if !okRole {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			users, err := authStore.ListUsers(r.Context())
			if err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			// include roles per user
			type userOut struct {
				ID        int64     `json:"id"`
				Email     string    `json:"email"`
				Name      string    `json:"name"`
				Picture   string    `json:"picture"`
				Provider  string    `json:"provider"`
				Subject   string    `json:"subject"`
				CreatedAt time.Time `json:"created_at"`
				UpdatedAt time.Time `json:"updated_at"`
				Roles     []string  `json:"roles"`
			}
			out := make([]userOut, 0, len(users))
			for _, u := range users {
				roles, _ := authStore.RolesForUser(r.Context(), u.ID)
				out = append(out, userOut{ID: u.ID, Email: u.Email, Name: u.Name, Picture: u.Picture, Provider: u.Provider, Subject: u.Subject, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, Roles: roles})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			// Admin only
			if u, ok := auth.CurrentUser(r.Context()); ok {
				okRole, _ := authStore.HasRole(r.Context(), u.ID, "admin")
				if !okRole {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var in struct {
				Email, Name, Picture, Provider, Subject string
				Roles                                   []string
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			u := &auth.User{Email: in.Email, Name: in.Name, Picture: in.Picture, Provider: in.Provider, Subject: in.Subject}
			u, err := authStore.UpsertUser(r.Context(), u)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = authStore.SetUserRoles(r.Context(), u.ID, in.Roles)
			roles, _ := authStore.RolesForUser(r.Context(), u.ID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "picture": u.Picture, "provider": u.Provider, "subject": u.Subject, "created_at": u.CreatedAt, "updated_at": u.UpdatedAt, "roles": roles})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/users/", func(w http.ResponseWriter, r *http.Request) {
		if !cfg.Auth.Enabled || authStore == nil {
			http.NotFound(w, r)
			return
		}
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		// Extract id
		idStr := strings.TrimPrefix(r.URL.Path, "/api/users/")
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			http.NotFound(w, r)
			return
		}
		var id int64
		if _, err := fmt.Sscan(idStr, &id); err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}

		// Admin check for mutating methods
		isAdmin := false
		if u, ok := auth.CurrentUser(r.Context()); ok {
			okRole, _ := authStore.HasRole(r.Context(), u.ID, "admin")
			if okRole {
				isAdmin = true
			}
		}

		switch r.Method {
		case http.MethodGet:
			// Admin only
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			u, err := authStore.GetUserByID(r.Context(), id)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			roles, _ := authStore.RolesForUser(r.Context(), u.ID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "picture": u.Picture, "provider": u.Provider, "subject": u.Subject, "created_at": u.CreatedAt, "updated_at": u.UpdatedAt, "roles": roles})
		case http.MethodPut:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var in struct {
				Email, Name, Picture, Provider, Subject string
				Roles                                   []string
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			u := &auth.User{ID: id, Email: in.Email, Name: in.Name, Picture: in.Picture, Provider: in.Provider, Subject: in.Subject}
			if err := authStore.UpdateUser(r.Context(), u); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = authStore.SetUserRoles(r.Context(), id, in.Roles)
			roles, _ := authStore.RolesForUser(r.Context(), id)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": id, "email": in.Email, "name": in.Name, "picture": in.Picture, "provider": in.Provider, "subject": in.Subject, "roles": roles})
		case http.MethodDelete:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if err := authStore.DeleteUser(r.Context(), id); err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Specialists store: prefer Postgres if configured, else memory
	var specStore persist.SpecialistsStore
	{
		// Use default DB DSN if available
		var pg *pgxpool.Pool
		if cfg.Databases.DefaultDSN != "" {
			if p, err := databasesTestPool(context.Background(), cfg.Databases.DefaultDSN); err == nil {
				pg = p
			}
		}
		specStore = databases.NewSpecialistsStore(pg)
		_ = specStore.Init(context.Background())
		// Seed from YAML only if the store is empty or missing entries
		if list, err := specStore.List(context.Background()); err == nil {
			existing := map[string]bool{}
			for _, s := range list {
				existing[s.Name] = true
			}
			for _, sc := range cfg.Specialists {
				if sc.Name == "" {
					continue
				}
				if existing[sc.Name] {
					continue
				}
				_, _ = specStore.Upsert(context.Background(), persist.Specialist{
					Name: sc.Name, BaseURL: sc.BaseURL, APIKey: sc.APIKey, Model: sc.Model,
					EnableTools: sc.EnableTools, Paused: sc.Paused, AllowTools: sc.AllowTools,
					ReasoningEffort: sc.ReasoningEffort, System: sc.System,
					ExtraHeaders: sc.ExtraHeaders, ExtraParams: sc.ExtraParams,
				})
			}
		}
		// Ensure tool registry reflects persisted store on startup
		if list, err := specStore.List(context.Background()); err == nil {
			specReg.ReplaceFromConfigs(cfg.OpenAI, specialistsFromStore(list), httpClient, baseToolRegistry)
		}

		// Load orchestrator configuration from the database (if present),
		// otherwise ensure a safe default system prompt is applied.
		if sp, ok, _ := specStore.GetByName(context.Background(), "orchestrator"); ok {
			// Apply DB-backed orchestrator settings to runtime config
			cfg.OpenAI.BaseURL = sp.BaseURL
			cfg.OpenAI.APIKey = sp.APIKey
			if strings.TrimSpace(sp.Model) != "" {
				cfg.OpenAI.Model = sp.Model
			}
			cfg.EnableTools = sp.EnableTools
			cfg.ToolAllowList = append([]string(nil), sp.AllowTools...)
			if strings.TrimSpace(sp.System) != "" {
				cfg.SystemPrompt = sp.System
			} else {
				cfg.SystemPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."
			}
			if sp.ExtraHeaders != nil {
				cfg.OpenAI.ExtraHeaders = sp.ExtraHeaders
			}
			if sp.ExtraParams != nil {
				cfg.OpenAI.ExtraParams = sp.ExtraParams
			}
			// Rebuild the LLM client for orchestrator and update tool exposure
			llm = openaillm.New(cfg.OpenAI, httpClient)
			eng.LLM = llm
			eng.Model = cfg.OpenAI.Model
			eng.System = prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt)
			// Select the appropriate tool registry for orchestrator only
			if !cfg.EnableTools {
				toolRegistry = tools.NewRegistry()
			} else if len(cfg.ToolAllowList) > 0 {
				toolRegistry = tools.NewFilteredRegistry(baseToolRegistry, cfg.ToolAllowList)
			} else {
				toolRegistry = baseToolRegistry
			}
			// Update engine and workflow runner to reflect the new registry
			eng.Tools = toolRegistry
			warppMu.Lock()
			warppRunner.Tools = toolRegistry
			warppMu.Unlock()
			// Also log the active tools for observability
			{
				names := make([]string, 0, len(toolRegistry.Schemas()))
				for _, s := range toolRegistry.Schemas() {
					names = append(names, s.Name)
				}
				log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents_loaded_from_db")
			}
		} else {
			// No DB record for orchestrator. Default to the required fallback system prompt
			// and reflect it in the engine regardless of any YAML/env value.
			cfg.SystemPrompt = "You are a helpful assistant with access to tools and specialists to help you complete objectives."
			eng.System = prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt)
		}
	}

	// Helper to render the main orchestrator as a synthetic specialist for the UI only.
	orchSpec := func() persist.Specialist {
		return persist.Specialist{
			ID:              0,
			Name:            "orchestrator",
			BaseURL:         cfg.OpenAI.BaseURL,
			APIKey:          cfg.OpenAI.APIKey,
			Model:           cfg.OpenAI.Model,
			EnableTools:     cfg.EnableTools,
			Paused:          false,
			AllowTools:      cfg.ToolAllowList,
			ReasoningEffort: "",
			System:          cfg.SystemPrompt,
			ExtraHeaders:    cfg.OpenAI.ExtraHeaders,
			ExtraParams:     cfg.OpenAI.ExtraParams,
		}
	}

	// Status endpoint: reflect specialists as agents for UI
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		type agent struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			State     string `json:"state"`
			Model     string `json:"model"`
			UpdatedAt string `json:"updatedAt"`
		}
		list, _ := specStore.List(r.Context())
		out := make([]agent, 0, len(list))
		now := time.Now().UTC().Format(time.RFC3339)
		for _, s := range list {
			if s.Paused {
				continue
			}
			state := "online"
			out = append(out, agent{ID: s.Name, Name: s.Name, State: state, Model: s.Model, UpdatedAt: now})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	// Specialists CRUD: list/create/update/delete/pause
	mux.HandleFunc("/api/specialists", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		// CORS: allow cross-origin UI
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			list, err := specStore.List(r.Context())
			if err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			// Append or replace with a synthetic orchestrator entry for frontend editing.
			found := false
			for i := range list {
				if strings.EqualFold(strings.TrimSpace(list[i].Name), "orchestrator") {
					list[i] = orchSpec()
					found = true
					break
				}
			}
			if !found {
				list = append(list, orchSpec())
			}
			json.NewEncoder(w).Encode(list)
		case http.MethodPost:
			// admin-only when auth enabled
			if cfg.Auth.Enabled {
				if u, ok := auth.CurrentUser(r.Context()); ok {
					okRole, err := authStore.HasRole(r.Context(), u.ID, "admin")
					if err != nil || !okRole {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var sp persist.Specialist
			if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			// Prevent creating a synthetic orchestrator via POST; require PUT to the named endpoint.
			if strings.EqualFold(strings.TrimSpace(sp.Name), "orchestrator") {
				http.Error(w, "cannot create orchestrator; use PUT /api/specialists/orchestrator", http.StatusBadRequest)
				return
			}
			saved, err := specStore.Upsert(r.Context(), sp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// Rebuild specialists registry after change (independent of orchestrator allow-list)
			if list, err := specStore.List(r.Context()); err == nil {
				specReg.ReplaceFromConfigs(cfg.OpenAI, specialistsFromStore(list), httpClient, baseToolRegistry)
			}
			json.NewEncoder(w).Encode(saved)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/specialists/", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		// CORS: allow cross-origin UI
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/specialists/")
		name = strings.TrimSpace(name)
		if name == "" {
			http.NotFound(w, r)
			return
		}
		// admin-only for mutating
		isAdmin := true
		if cfg.Auth.Enabled {
			isAdmin = false
			if u, ok := auth.CurrentUser(r.Context()); ok {
				okRole, err := authStore.HasRole(r.Context(), u.ID, "admin")
				if err == nil && okRole {
					isAdmin = true
				}
			}
		}
		switch r.Method {
		case http.MethodGet:
			if name == "orchestrator" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(orchSpec())
				return
			}
			sp, ok, _ := specStore.GetByName(r.Context(), name)
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sp)
		case http.MethodPut:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var sp persist.Specialist
			if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if name == "orchestrator" {
				// Apply updates to the main orchestrator configuration and persist to DB.
				cfg.OpenAI.BaseURL = sp.BaseURL
				cfg.OpenAI.APIKey = sp.APIKey
				if strings.TrimSpace(sp.Model) != "" {
					cfg.OpenAI.Model = sp.Model
				}
				cfg.EnableTools = sp.EnableTools
				cfg.ToolAllowList = append([]string(nil), sp.AllowTools...)
				cfg.SystemPrompt = sp.System
				if sp.ExtraHeaders != nil {
					cfg.OpenAI.ExtraHeaders = sp.ExtraHeaders
				}
				if sp.ExtraParams != nil {
					cfg.OpenAI.ExtraParams = sp.ExtraParams
				}
				// Rebuild the LLM client for orchestrator and update tool exposure
				llm = openaillm.New(cfg.OpenAI, httpClient)
				eng.LLM = llm
				eng.Model = cfg.OpenAI.Model
				eng.System = prompts.DefaultSystemPrompt(cfg.Workdir, cfg.SystemPrompt)
				// Select the appropriate tool registry for orchestrator only
				if !cfg.EnableTools {
					toolRegistry = tools.NewRegistry()
				} else if len(cfg.ToolAllowList) > 0 {
					toolRegistry = tools.NewFilteredRegistry(baseToolRegistry, cfg.ToolAllowList)
				} else {
					toolRegistry = baseToolRegistry
				}
				// Update engine and workflow runner to reflect the new registry
				eng.Tools = toolRegistry
				warppMu.Lock()
				warppRunner.Tools = toolRegistry
				warppMu.Unlock()
				// Persist orchestrator configuration to specialists store
				toSave := persist.Specialist{
					Name:            "orchestrator",
					BaseURL:         cfg.OpenAI.BaseURL,
					APIKey:          cfg.OpenAI.APIKey,
					Model:           cfg.OpenAI.Model,
					EnableTools:     cfg.EnableTools,
					Paused:          false,
					AllowTools:      append([]string(nil), cfg.ToolAllowList...),
					ReasoningEffort: sp.ReasoningEffort,
					System:          cfg.SystemPrompt,
					ExtraHeaders:    cfg.OpenAI.ExtraHeaders,
					ExtraParams:     cfg.OpenAI.ExtraParams,
				}
				if _, err := specStore.Upsert(r.Context(), toSave); err != nil {
					log.Error().Err(err).Msg("failed to persist orchestrator configuration")
					http.Error(w, "failed to persist orchestrator configuration", http.StatusInternalServerError)
					return
				}
				// Keep specialists registry in sync (orchestrator is filtered out downstream)
				if list, err := specStore.List(r.Context()); err == nil {
					specReg.ReplaceFromConfigs(cfg.OpenAI, specialistsFromStore(list), httpClient, baseToolRegistry)
				}
				// Return the updated synthetic specialist
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(orchSpec())
				// Also log the active tools for observability
				{
					names := make([]string, 0, len(toolRegistry.Schemas()))
					for _, s := range toolRegistry.Schemas() {
						names = append(names, s.Name)
					}
					log.Info().Bool("enableTools", cfg.EnableTools).Strs("allowList", cfg.ToolAllowList).Strs("tools", names).Msg("tool_registry_contents_updated")
				}
				return
			}
			// Regular specialist update path
			sp.Name = name // enforce path name
			saved, err := specStore.Upsert(r.Context(), sp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(saved)
			if list, err := specStore.List(r.Context()); err == nil {
				specReg.ReplaceFromConfigs(cfg.OpenAI, specialistsFromStore(list), httpClient, baseToolRegistry)
			}
		case http.MethodDelete:
			if !isAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if name == "orchestrator" {
				http.Error(w, "cannot delete orchestrator", http.StatusBadRequest)
				return
			}
			if err := specStore.Delete(r.Context(), name); err != nil {
				http.Error(w, "error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			if list, err := specStore.List(r.Context()); err == nil {
				specReg.ReplaceFromConfigs(cfg.OpenAI, specialistsFromStore(list), httpClient, baseToolRegistry)
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Simple in-process metrics endpoint for web UI token graph.
	mux.HandleFunc("/api/metrics/tokens", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"timestamp": time.Now().Unix(), "models": llmpkg.TokenTotalsSnapshot()})
	})

	mux.HandleFunc("/api/warpp/tools", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Always expose the full, unfiltered tool registry here so that
		// the UI can configure per-specialist allow-lists independently
		// of the orchestrator's current allow-list/filtering.
		schemas := baseToolRegistry.Schemas()
		out := make([]map[string]any, 0, len(schemas))
		for _, s := range schemas {
			out = append(out, map[string]any{
				"name":        s.Name,
				"description": s.Description,
				"parameters":  s.Parameters,
			})
		}
		json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/api/warpp/workflows", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		warppMu.RLock()
		list := wfreg.All()
		warppMu.RUnlock()
		sort.Slice(list, func(i, j int) bool { return list[i].Intent < list[j].Intent })
		json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("/api/warpp/workflows/", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		intent := strings.TrimPrefix(r.URL.Path, "/api/warpp/workflows/")
		intent = strings.TrimSpace(intent)
		if intent == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			warppMu.RLock()
			wf, err := wfreg.Get(intent)
			warppMu.RUnlock()
			if err != nil {
				http.Error(w, "workflow not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(wf)
		case http.MethodPut:
			if cfg.Auth.Enabled {
				if u, ok := auth.CurrentUser(r.Context()); ok {
					okRole, err := authStore.HasRole(r.Context(), u.ID, "admin")
					if err != nil || !okRole {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()
			var wf warpp.Workflow
			if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if wf.Intent == "" {
				wf.Intent = intent
			}
			if wf.Intent != intent {
				http.Error(w, "intent mismatch", http.StatusBadRequest)
				return
			}
			if len(wf.Steps) == 0 {
				http.Error(w, "workflow requires steps", http.StatusBadRequest)
				return
			}
			seen := make(map[string]struct{}, len(wf.Steps))
			for _, step := range wf.Steps {
				if step.ID == "" {
					http.Error(w, "step id required", http.StatusBadRequest)
					return
				}
				if _, ok := seen[step.ID]; ok {
					http.Error(w, "duplicate step id", http.StatusBadRequest)
					return
				}
				seen[step.ID] = struct{}{}
				if step.Tool != nil && step.Tool.Name == "" {
					http.Error(w, "tool name required", http.StatusBadRequest)
					return
				}
			}
			// Persist to DB-backed store (no filesystem writes)
			_, existed, _ := wfStore.Get(r.Context(), intent)
			var pw persist.WarppWorkflow
			if b, err := json.Marshal(wf); err == nil {
				_ = json.Unmarshal(b, &pw)
			}
			if _, err := wfStore.Upsert(r.Context(), pw); err != nil {
				http.Error(w, "failed to save workflow", http.StatusInternalServerError)
				return
			}
			// Update in-memory registry
			warppMu.Lock()
			if wfreg == nil {
				wfreg = &warpp.Registry{}
			}
			wfreg.Upsert(wf, "")
			warppRunner.Workflows = wfreg
			warppMu.Unlock()
			status := http.StatusOK
			if !existed {
				status = http.StatusCreated
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(wf)
		case http.MethodDelete:
			// Admin-only when auth enabled
			if cfg.Auth.Enabled {
				if u, ok := auth.CurrentUser(r.Context()); ok {
					okRole, err := authStore.HasRole(r.Context(), u.ID, "admin")
					if err != nil || !okRole {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
			if err := wfStore.Delete(r.Context(), intent); err != nil {
				http.Error(w, "failed to delete", http.StatusInternalServerError)
				return
			}
			warppMu.Lock()
			if wfreg != nil {
				wfreg.Remove(intent)
				warppRunner.Workflows = wfreg
			}
			warppMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// POST /api/warpp/run executes a workflow by intent and returns {result: "..."}
	mux.HandleFunc("/api/warpp/run", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Limit body size
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()
		var req struct {
			Intent string `json:"intent"`
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		intent := strings.TrimSpace(req.Intent)
		if intent == "" {
			http.Error(w, "intent required", http.StatusBadRequest)
			return
		}
		// Lookup workflow
		warppMu.RLock()
		wf, err := wfreg.Get(intent)
		warppMu.RUnlock()
		if err != nil {
			http.Error(w, "workflow not found", http.StatusNotFound)
			return
		}
		// Build attributes and allow-list
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			prompt = "(ui) run workflow"
		}
		attrs := warpp.Attrs{"utter": prompt}
		// Personalize trims by guards; produce allow-list from steps
		seconds := cfg.WorkflowTimeoutSeconds
		if seconds <= 0 {
			seconds = cfg.AgentRunTimeoutSeconds
		}
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/api/warpp/run").Msg("using configured workflow timeout")
		} else {
			log.Debug().Str("endpoint", "/api/warpp/run").Msg("no timeout configured; running until completion")
		}
		wfStar, _, attrs2, err := warppRunner.Personalize(ctx, wf, attrs)
		if err != nil {
			log.Error().Err(err).Msg("warpp_personalize")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		allow := map[string]bool{}
		for _, s := range wfStar.Steps {
			if s.Tool != nil {
				allow[s.Tool.Name] = true
			}
		}
		result, trace, err := warppRunner.ExecuteWithTrace(ctx, wfStar, allow, attrs2, nil)
		if err != nil {
			log.Error().Err(err).Msg("warpp_execute")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "trace": trace})
	})

	mux.HandleFunc("/agent/run", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Prompt    string `json:"prompt"`
			SessionID string `json:"session_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Create run record (running)
		currentRun := runs.create(req.Prompt)

		// Use default session if not provided
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		// Ensure persistent session exists and load prior history
		if _, err := ensureChatSession(r.Context(), chatStore, req.SessionID); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("ensure_chat_session")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		history, err := chatMemory.BuildContext(r.Context(), req.SessionID)
		if err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("load_chat_history")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Pre-dispatch routing: call a specialist directly if there's a match.
		if name := specialists.Route(cfg.SpecialistRoutes, req.Prompt); name != "" {
			log.Info().Str("route", name).Msg("pre-dispatch specialist route matched")
			a, ok := specReg.Get(name)
			if !ok {
				log.Error().Str("route", name).Msg("specialist not found for route")
			} else {
				seconds := cfg.AgentRunTimeoutSeconds
				ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
				defer cancel()
				if dur > 0 {
					log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("mode", "specialist_pre_dispatch").Msg("using configured agent timeout")
				} else {
					log.Debug().Str("endpoint", "/agent/run").Str("mode", "specialist_pre_dispatch").Msg("no timeout configured; running until completion")
				}
				out, err := a.Inference(ctx, req.Prompt, nil)
				if err != nil {
					log.Error().Err(err).Msg("specialist pre-dispatch")
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"result": out})
				return
			}
		}

		// WARPP mode: run the WARPP workflow executor instead of the LLM loop
		if r.URL.Query().Get("warpp") == "true" {
			seconds := cfg.WorkflowTimeoutSeconds
			if seconds <= 0 {
				seconds = cfg.AgentRunTimeoutSeconds
			}
			ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
			defer cancel()
			if dur > 0 {
				log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Str("mode", "warpp").Msg("using configured workflow timeout")
			} else {
				log.Debug().Str("endpoint", "/agent/run").Str("mode", "warpp").Msg("no timeout configured; running until completion")
			}
			intent := warppRunner.DetectIntent(ctx, req.Prompt)
			wf, _ := wfreg.Get(intent)
			attrs := warpp.Attrs{"utter": req.Prompt}
			wfStar, _, attrs, err := warppRunner.Personalize(ctx, wf, attrs)
			if err != nil {
				log.Error().Err(err).Msg("personalize")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			allow := map[string]bool{}
			for _, s := range wfStar.Steps {
				if s.Tool != nil {
					allow[s.Tool.Name] = true
				}
			}
			final, err := warppRunner.Execute(ctx, wfStar, allow, attrs, nil)
			if err != nil {
				log.Error().Err(err).Msg("warpp")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"result": final})
			return
		}

		// If no OpenAI API key is configured, return a deterministic dev response
		// so the web UI can be exercised locally without external credentials.
		if cfg.OpenAI.APIKey == "" {
			// Support SSE if requested
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock response: " + req.Prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock response")
				}
				fl.Flush()
				runs.updateStatus(currentRun.ID, "completed", 0)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + req.Prompt})
			runs.updateStatus(currentRun.ID, "completed", 0)
			return
		}

		// If client requested SSE, use streaming RunStream and proxy deltas/tool events
		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			seconds := cfg.StreamRunTimeoutSeconds
			if seconds <= 0 {
				seconds = cfg.AgentRunTimeoutSeconds
			}
			ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
			defer cancel()
			if dur > 0 {
				log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Bool("stream", true).Msg("using configured stream timeout")
			} else {
				log.Debug().Str("endpoint", "/agent/run").Bool("stream", true).Msg("no timeout configured; running until completion")
			}

			// Wire up engine callbacks to write SSE events.
			// delta -> incremental assistant text
			eng.OnDelta = func(d string) {
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			// tool start -> announce tool invocation early (no result yet)
			eng.OnToolStart = func(name string, args []byte, toolID string) {
				payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			// tool result -> append results (and emit specialized tts_audio event for TTS playback)
			eng.OnTool = func(name string, args []byte, result []byte) {
				// Stream chunk event
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					b, _ := json.Marshal(metaPayload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()

				if name == "text_to_speech" {
					var resp map[string]any
					if err := json.Unmarshal(result, &resp); err == nil {
						if fp, ok := resp["file_path"].(string); ok && fp != "" {
							trimmed := strings.TrimPrefix(fp, "./")
							trimmed = strings.TrimPrefix(trimmed, "/")
							url := "/audio/" + trimmed
							ap := map[string]any{"type": "tts_audio", "file_path": fp, "url": url}
							if bb, err2 := json.Marshal(ap); err2 == nil {
								fmt.Fprintf(w, "data: %s\n\n", bb)
								fl.Flush()
							}
						}
					}
				}
			}

			// Run streaming engine
			res, err := eng.RunStream(ctx, req.Prompt, history)
			if err != nil {
				log.Error().Err(err).Msg("agent run error")
				if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
					fmt.Fprintf(w, "data: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "data: %q\n\n", "(error)")
				}
				fl.Flush()
				runs.updateStatus(currentRun.ID, "failed", 0)
				return
			}
			// send final event
			payload := map[string]string{"type": "final", "data": res}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			runs.updateStatus(currentRun.ID, "completed", 0)
			if err := storeChatTurn(r.Context(), chatStore, req.SessionID, req.Prompt, res, eng.Model); err != nil {
				log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
			}
			return
		}

		// Non-streaming path
		seconds := cfg.AgentRunTimeoutSeconds
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/agent/run").Bool("stream", false).Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/run").Bool("stream", false).Msg("no timeout configured; running until completion")
		}
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			runs.updateStatus(currentRun.ID, "failed", 0)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
		runs.updateStatus(currentRun.ID, "completed", 0)

		if err := storeChatTurn(r.Context(), chatStore, req.SessionID, req.Prompt, result, eng.Model); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
		}
	})

	// POST /agent/vision accepts multipart/form-data with fields:
	// - images: one or more files (image/png or image/jpeg)
	// - prompt: string
	// - session_id: optional string
	// Returns JSON {result: "..."} or streams SSE deltas/final like /agent/run when Accept: text/event-stream
	mux.HandleFunc("/agent/vision", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Limit total form size to 20MB
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		prompt := r.FormValue("prompt")
		sessionID := r.FormValue("session_id")
		if sessionID == "" {
			sessionID = "default"
		}
		if _, err := ensureChatSession(r.Context(), chatStore, sessionID); err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("ensure_chat_session")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		history, err := chatMemory.BuildContext(r.Context(), sessionID)
		if err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("load_chat_history")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Collect image files (field name can be "images" repeated or any files in multipart form)
		form := r.MultipartForm
		var files []*multipart.FileHeader
		if form != nil {
			// Prefer "images" field
			if fh := form.File["images"]; len(fh) > 0 {
				files = append(files, fh...)
			}
			// Also accept legacy "image" field
			if fh := form.File["image"]; len(fh) > 0 {
				files = append(files, fh...)
			}
		}
		if len(files) == 0 {
			http.Error(w, "no images provided", http.StatusBadRequest)
			return
		}

		// If no API key configured, return a dev mock
		if cfg.OpenAI.APIKey == "" {
			vrun := runs.create("[vision] " + prompt)
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock vision response: " + prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock vision response")
				}
				fl.Flush()
				runs.updateStatus(vrun.ID, "completed", 0)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock vision response: " + prompt})
			runs.updateStatus(vrun.ID, "completed", 0)
			return
		}

		// Read and validate images; build attachments for provider
		type imgAtt struct {
			mime string
			b64  string
		}
		var atts []imgAtt
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				http.Error(w, "file open", http.StatusBadRequest)
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				http.Error(w, "file read", http.StatusBadRequest)
				return
			}
			// Basic mime detection
			mt := http.DetectContentType(data)
			if mt != "image/png" && mt != "image/jpeg" && mt != "image/jpg" {
				http.Error(w, "unsupported image type", http.StatusBadRequest)
				return
			}
			// Normalize jpg
			if mt == "image/jpg" {
				mt = "image/jpeg"
			}
			atts = append(atts, imgAtt{mime: mt, b64: base64.StdEncoding.EncodeToString(data)})
		}

		// Build initial message list including history and current user prompt
		msgs := make([]llmpkg.Message, 0, len(history)+1)
		msgs = append(msgs, history...)
		msgs = append(msgs, llmpkg.Message{Role: "user", Content: prompt})

		// Non-streaming path for simplicity (vision responses are usually short). Configurable timeout.
		vSeconds := cfg.AgentRunTimeoutSeconds
		ctx, cancel, vDur := withMaybeTimeout(r.Context(), vSeconds)
		defer cancel()
		if vDur > 0 {
			log.Debug().Dur("timeout", vDur).Str("endpoint", "/agent/vision").Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/vision").Msg("no timeout configured; running until completion")
		}

		// Convert to openai.ImageAttachment slice
		images := make([]openaillm.ImageAttachment, 0, len(atts))
		for _, a := range atts {
			images = append(images, openaillm.ImageAttachment{MimeType: a.mime, Base64Data: a.b64})
		}

		vrun := runs.create("[vision] " + prompt)
		out, err := llm.ChatWithImageAttachments(ctx, msgs, images, nil, cfg.OpenAI.Model)
		if err != nil {
			log.Error().Err(err).Msg("vision chat error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			runs.updateStatus(vrun.ID, "failed", 0)
			return
		}

		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}
			// Emit final only (no deltas since we used non-streaming provider call)
			payload := map[string]string{"type": "final", "data": out.Content}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			runs.updateStatus(vrun.ID, "completed", 0)
			if err := storeChatTurn(r.Context(), chatStore, sessionID, prompt, out.Content, cfg.OpenAI.Model); err != nil {
				log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_vision_stream")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": out.Content})
		runs.updateStatus(vrun.ID, "completed", 0)
		if err := storeChatTurn(r.Context(), chatStore, sessionID, prompt, out.Content, cfg.OpenAI.Model); err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_vision")
		}
	})

	// POST /api/prompt accepts {"prompt":"..."} and runs the agent (for web UI compatibility)
	mux.HandleFunc("/api/prompt", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Enabled {
			if _, ok := auth.CurrentUser(r.Context()); !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		// Basic CORS support
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// limit body to 64KB
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()

		var req struct {
			Prompt    string `json:"prompt"`
			SessionID string `json:"session_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("decode prompt: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Use default session if not provided
		if req.SessionID == "" {
			req.SessionID = "default"
		}

		if _, err := ensureChatSession(r.Context(), chatStore, req.SessionID); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("ensure_chat_session")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		history, err := chatMemory.BuildContext(r.Context(), req.SessionID)
		if err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("load_chat_history")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// If no OpenAI API key is configured, return a deterministic dev response
		if cfg.OpenAI.APIKey == "" {
			prun := runs.create(req.Prompt)
			// Support SSE if requested
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock response: " + req.Prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock response")
				}
				fl.Flush()
				runs.updateStatus(prun.ID, "completed", 0)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock response: " + req.Prompt})
			runs.updateStatus(prun.ID, "completed", 0)
			return
		}

		// If client requested SSE, use streaming RunStream and proxy deltas/tool events
		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}

			seconds := cfg.StreamRunTimeoutSeconds
			if seconds <= 0 {
				seconds = cfg.AgentRunTimeoutSeconds
			}
			ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
			defer cancel()
			if dur > 0 {
				log.Debug().Dur("timeout", dur).Str("endpoint", "/api/prompt").Bool("stream", true).Msg("using configured stream timeout")
			} else {
				log.Debug().Str("endpoint", "/api/prompt").Bool("stream", true).Msg("no timeout configured; running until completion")
			}

			// Wire up engine callbacks to write SSE events (duplicate path for /api/prompt).
			eng.OnDelta = func(d string) {
				payload := map[string]string{"type": "delta", "data": d}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnToolStart = func(name string, args []byte, toolID string) {
				payload := map[string]any{"type": "tool_start", "title": "Tool: " + name, "tool_id": toolID, "args": string(args)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			}
			eng.OnTool = func(name string, args []byte, result []byte) {
				if name == "text_to_speech_chunk" {
					var meta map[string]any
					_ = json.Unmarshal(result, &meta)
					metaPayload := map[string]any{"type": "tts_chunk", "bytes": meta["bytes"], "b64": meta["b64"]}
					b, _ := json.Marshal(metaPayload)
					fmt.Fprintf(w, "data: %s\n\n", b)
					fl.Flush()
					return
				}
				payload := map[string]any{"type": "tool_result", "title": "Tool: " + name, "data": string(result)}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
				if name == "text_to_speech" {
					var resp map[string]any
					if err := json.Unmarshal(result, &resp); err == nil {
						if fp, ok := resp["file_path"].(string); ok && fp != "" {
							trimmed := strings.TrimPrefix(fp, "./")
							trimmed = strings.TrimPrefix(trimmed, "/")
							url := "/audio/" + trimmed
							ap := map[string]any{"type": "tts_audio", "file_path": fp, "url": url}
							if bb, err2 := json.Marshal(ap); err2 == nil {
								fmt.Fprintf(w, "data: %s\n\n", bb)
								fl.Flush()
							}
						}
					}
				}
			}

			// Run streaming engine
			prun := runs.create(req.Prompt)
			res, err := eng.RunStream(ctx, req.Prompt, history)
			if err != nil {
				log.Error().Err(err).Msg("agent run error")
				if b, err2 := json.Marshal("(error) " + err.Error()); err2 == nil {
					fmt.Fprintf(w, "data: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "data: %q\n\n", "(error)")
				}
				fl.Flush()
				runs.updateStatus(prun.ID, "failed", 0)
				return
			}
			// send final event
			payload := map[string]string{"type": "final", "data": res}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			runs.updateStatus(prun.ID, "completed", 0)
			if err := storeChatTurn(r.Context(), chatStore, req.SessionID, req.Prompt, res, eng.Model); err != nil {
				log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn_stream")
			}
			return
		}

		// Non-streaming path
		seconds := cfg.AgentRunTimeoutSeconds
		ctx, cancel, dur := withMaybeTimeout(r.Context(), seconds)
		defer cancel()
		if dur > 0 {
			log.Debug().Dur("timeout", dur).Str("endpoint", "/api/prompt").Bool("stream", false).Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/api/prompt").Bool("stream", false).Msg("no timeout configured; running until completion")
		}
		prun := runs.create(req.Prompt)
		result, err := eng.Run(ctx, req.Prompt, history)
		if err != nil {
			log.Error().Err(err).Msg("agent run error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			runs.updateStatus(prun.ID, "failed", 0)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
		runs.updateStatus(prun.ID, "completed", 0)
		if err := storeChatTurn(r.Context(), chatStore, req.SessionID, req.Prompt, result, eng.Model); err != nil {
			log.Error().Err(err).Str("session", req.SessionID).Msg("store_chat_turn")
		}
	})

	// Serve TTS audio files
	mux.HandleFunc("/audio/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Extract filename from path
		filename := strings.TrimPrefix(r.URL.Path, "/audio/")
		if filename == "" {
			http.Error(w, "file not specified", http.StatusBadRequest)
			return
		}
		// Serve the file from the working directory (where TTS saves files)
		http.ServeFile(w, r, filename)
	})

	// POST /stt accepts multipart/form-data with field "audio" (WAV 16kHz mono or stereo) and returns {text: "..."}
	mux.HandleFunc("/stt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if whisperModel == nil { // model failed to load
			http.Error(w, "whisper model unavailable", http.StatusServiceUnavailable)
			return
		}
		// 32MB max upload
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("audio")
		if err != nil {
			http.Error(w, "missing audio", http.StatusBadRequest)
			return
		}
		defer file.Close()
		// Read whole file (size bounded by ParseMultipartForm limit)
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		// Minimal WAV parsing (reuse logic similar to whisper-go example)
		if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
			http.Error(w, "unsupported audio (expect WAV)", http.StatusBadRequest)
			return
		}
		// Extract format fields
		channels := binary.LittleEndian.Uint16(data[22:24])
		sampleRate := binary.LittleEndian.Uint32(data[24:28])
		bitsPerSample := binary.LittleEndian.Uint16(data[34:36])
		// Find "data" chunk (simple scan)
		offset := 12
		var audioStart, audioLen int
		for offset+8 <= len(data) {
			chunkID := string(data[offset : offset+4])
			chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			if chunkID == "data" {
				audioStart = offset + 8
				audioLen = chunkSize
				break
			}
			offset += 8 + chunkSize
		}
		if audioLen == 0 || audioStart+audioLen > len(data) {
			http.Error(w, "invalid wav data", http.StatusBadRequest)
			return
		}
		raw := data[audioStart : audioStart+audioLen]
		var samples []float32
		switch bitsPerSample {
		case 16:
			for i := 0; i+1 < len(raw); i += 2 {
				sample := int16(binary.LittleEndian.Uint16(raw[i : i+2]))
				samples = append(samples, float32(sample)/32768.0)
			}
		case 32:
			for i := 0; i+3 < len(raw); i += 4 {
				bits := binary.LittleEndian.Uint32(raw[i : i+4])
				f := *(*float32)(unsafe.Pointer(&bits))
				samples = append(samples, f)
			}
		default:
			http.Error(w, "unsupported bit depth", http.StatusBadRequest)
			return
		}
		if channels == 2 { // stereo -> mono
			mono := make([]float32, len(samples)/2)
			for i := 0; i < len(mono); i++ {
				mono[i] = (samples[i*2] + samples[i*2+1]) / 2
			}
			samples = mono
		}
		if sampleRate != 16000 {
			log.Warn().Uint32("rate", sampleRate).Msg("non-16k audio provided; transcription may be degraded")
		}
		ctx, err := whisperModel.NewContext()
		if err != nil {
			http.Error(w, "ctx error", http.StatusInternalServerError)
			return
		}
		ctx.SetLanguage("en")
		if err := ctx.Process(samples, nil, nil, nil); err != nil {
			http.Error(w, "process error", http.StatusInternalServerError)
			return
		}
		var sb strings.Builder
		for {
			seg, err := ctx.NextSegment()
			if err != nil {
				break
			}
			sb.WriteString(seg.Text)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": strings.TrimSpace(sb.String())})
	})

	frontendProxy := os.Getenv("FRONTEND_DEV_PROXY")
	uiOpts := webui.Options{DevProxy: frontendProxy}
	if cfg.Auth.Enabled {
		uiOpts.AuthGate = func(r *http.Request) bool {
			_, ok := auth.CurrentUser(r.Context())
			return ok
		}
		uiOpts.UnauthedRedirect = "/auth/login"
	}
	if err := webui.RegisterFrontend(mux, uiOpts); err != nil {
		log.Error().Err(err).Msg("frontend registration failed")
	}
	if frontendProxy != "" {
		log.Info().Str("url", frontendProxy).Msg("frontend dev proxy enabled")
	}

	// Wrap with auth middleware to attach user to context if enabled
	var root http.Handler = mux
	if cfg.Auth.Enabled && authStore != nil {
		root = auth.Middleware(authStore, cfg.Auth.CookieName, false)(mux)
	}

	log.Info().Msg("agentd listening on :32180")
	if err := http.ListenAndServe(":32180", root); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

// databasesTestPool is a small wrapper to reuse the existing newPgPool from internal/persistence/databases.
// We can't import unexported symbol, so replicate minimal connection here for auth.
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

// formatToolPayload helper is defined and used in internal/tui; no duplicate needed here.

// specialistsFromStore converts persisted specialists to config structs
func specialistsFromStore(list []persist.Specialist) []config.SpecialistConfig {
	out := make([]config.SpecialistConfig, 0, len(list))
	for _, s := range list {
		// Skip the orchestrator; it is the main agent, not a specialist tool
		if strings.EqualFold(strings.TrimSpace(s.Name), "orchestrator") {
			continue
		}
		out = append(out, config.SpecialistConfig{
			Name: s.Name, BaseURL: s.BaseURL, APIKey: s.APIKey, Model: s.Model,
			EnableTools: s.EnableTools, Paused: s.Paused, AllowTools: s.AllowTools,
			ReasoningEffort: s.ReasoningEffort, System: s.System,
			ExtraHeaders: s.ExtraHeaders, ExtraParams: s.ExtraParams,
		})
	}
	return out
}
