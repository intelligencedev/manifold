package databases

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"manifold/internal/agent/memory/belief"
	"manifold/internal/agent/memory/decision"
	"manifold/internal/config"
	"manifold/internal/durable"
	"manifold/internal/persistence"
	sqlitep "manifold/internal/persistence/sqlite"
	"manifold/internal/secrets"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewManager constructs database backends based on configuration.
// Supported backends: sqlite, memory, none, auto, postgres.
func NewManager(ctx context.Context, cfg config.DBConfig) (m Manager, err error) {
	defer func() {
		if err != nil {
			m.Close()
		}
	}()

	// Resolve DSNs with default fallback
	searchDSN := firstNonEmpty(cfg.Search.DSN, cfg.DefaultDSN)
	vectorDSN := firstNonEmpty(cfg.Vector.DSN, cfg.DefaultDSN)
	graphDSN := firstNonEmpty(cfg.Graph.DSN, cfg.DefaultDSN)
	chatDSN := firstNonEmpty(cfg.Chat.DSN, cfg.DefaultDSN)

	rootBackend := resolveRootBackend(cfg)
	searchBackend := resolveBackend(cfg.Search.Backend, rootBackend)
	vectorBackend := resolveBackend(cfg.Vector.Backend, rootBackend)
	graphBackend := resolveBackend(cfg.Graph.Backend, rootBackend)
	chatBackend := resolveBackend(cfg.Chat.Backend, rootBackend)

	if needsSQLite(rootBackend, searchBackend, vectorBackend, graphBackend, chatBackend) {
		m.SQLite, err = openSQLite(ctx, cfg.SQLite)
		if err != nil {
			return Manager{}, err
		}
	}

	m.Search, err = buildSearchStore(ctx, searchBackend, searchDSN, m.SQLite)
	if err != nil {
		return Manager{}, err
	}

	vectorCfg := cfg.Vector
	vectorCfg.Backend = vectorBackend
	m.Vector, err = buildVectorStore(ctx, vectorCfg, vectorDSN, m.SQLite, cfg.SQLite.Vector)
	if err != nil {
		return Manager{}, err
	}

	m.Graph, err = buildGraphStore(ctx, graphBackend, graphDSN, m.SQLite)
	if err != nil {
		return Manager{}, err
	}

	m.Chat, err = buildChatStore(ctx, chatBackend, chatDSN, m.SQLite)
	if err != nil {
		return Manager{}, err
	}

	if m.Chat == nil {
		m.Chat = newMemoryChatStore()
	}
	if err := initStore(ctx, "chat store", m.Chat); err != nil {
		return Manager{}, err
	}

	m.SpecialistActivity, err = buildSpecialistActivityStore(ctx, chatBackend, chatDSN, m.SQLite)
	if err != nil {
		return Manager{}, err
	}
	if m.SpecialistActivity == nil {
		m.SpecialistActivity = NewMemorySpecialistActivityStore()
	}
	if err := initStore(ctx, "specialist activity store", m.SpecialistActivity); err != nil {
		return Manager{}, err
	}

	if err := initializeDefaultStores(ctx, &m, cfg, chatDSN); err != nil {
		return Manager{}, err
	}

	return m, nil
}

func buildSearchStore(ctx context.Context, backend, dsn string, sqliteDB *sql.DB) (FullTextSearch, error) {
	switch backend {
	case "", "memory":
		return NewMemorySearch(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresSearch(pool), nil
		}
		if sqliteDB != nil {
			return NewSQLiteSearch(sqliteDB), nil
		}
		return NewMemorySearch(), nil
	case "sqlite":
		if sqliteDB == nil {
			return nil, fmt.Errorf("search backend sqlite requires database")
		}
		return NewSQLiteSearch(sqliteDB), nil
	case "postgres", "pg":
		if dsn == "" {
			return nil, fmt.Errorf("search backend postgres requires DSN")
		}
		pool, err := newPgPool(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("connect postgres (search): %w", err)
		}
		return NewPostgresSearch(pool), nil
	case "none", "disabled":
		return noopSearch{}, nil
	default:
		return nil, fmt.Errorf("unsupported search backend: %s", backend)
	}
}

func buildVectorStore(ctx context.Context, cfg config.VectorConfig, dsn string, sqliteDB *sql.DB, sqliteCfg config.SQLiteVectorConfig) (VectorStore, error) {
	switch cfg.Backend {
	case "", "memory":
		return NewMemoryVector(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			if err := ensureVectorExtension(ctx, pool); err != nil {
				pool.Close()
				return NewMemoryVector(), nil
			}
			return NewPostgresVector(pool, cfg.Dimensions, cfg.Metric), nil
		}
		if sqliteDB != nil {
			return NewSQLiteVector(sqliteDB, cfg, sqliteCfg)
		}
		return NewMemoryVector(), nil
	case "sqlite":
		if sqliteDB == nil {
			return nil, fmt.Errorf("vector backend sqlite requires database")
		}
		return NewSQLiteVector(sqliteDB, cfg, sqliteCfg)
	case "postgres", "pgvector", "pg":
		if dsn == "" {
			return nil, fmt.Errorf("vector backend postgres requires DSN")
		}
		pool, err := newPgPool(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("connect postgres (vector): %w", err)
		}
		if err := ensureVectorExtension(ctx, pool); err != nil {
			pool.Close()
			return nil, fmt.Errorf("connect postgres (vector): %w", err)
		}
		return NewPostgresVector(pool, cfg.Dimensions, cfg.Metric), nil
	case "qdrant":
		if dsn == "" {
			return nil, fmt.Errorf("vector backend qdrant requires DSN")
		}
		store, err := NewQdrantVector(dsn, cfg.Index, cfg.Dimensions, cfg.Metric)
		if err != nil {
			return nil, fmt.Errorf("connect qdrant (vector): %w", err)
		}
		return store, nil
	case "none", "disabled":
		return noopVector{}, nil
	default:
		return nil, fmt.Errorf("unsupported vector backend: %s", cfg.Backend)
	}
}

func buildGraphStore(ctx context.Context, backend, dsn string, sqliteDB *sql.DB) (GraphDB, error) {
	switch backend {
	case "", "memory":
		return NewMemoryGraph(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresGraph(pool), nil
		}
		if sqliteDB != nil {
			return NewSQLiteGraph(sqliteDB), nil
		}
		return NewMemoryGraph(), nil
	case "sqlite":
		if sqliteDB == nil {
			return nil, fmt.Errorf("graph backend sqlite requires database")
		}
		return NewSQLiteGraph(sqliteDB), nil
	case "postgres", "pg":
		if dsn == "" {
			return nil, fmt.Errorf("graph backend postgres requires DSN")
		}
		pool, err := newPgPool(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("connect postgres (graph): %w", err)
		}
		return NewPostgresGraph(pool), nil
	case "none", "disabled":
		return noopGraph{}, nil
	default:
		return nil, fmt.Errorf("unsupported graph backend: %s", backend)
	}
}

func buildChatStore(ctx context.Context, backend, dsn string, sqliteDB *sql.DB) (persistence.ChatStore, error) {
	switch backend {
	case "", "memory", "none", "disabled":
		return newMemoryChatStore(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresChatStore(pool), nil
		}
		if sqliteDB != nil {
			return NewSQLiteChatStore(sqliteDB), nil
		}
		return newMemoryChatStore(), nil
	case "sqlite":
		if sqliteDB == nil {
			return nil, fmt.Errorf("chat backend sqlite requires database")
		}
		return NewSQLiteChatStore(sqliteDB), nil
	case "postgres", "pg":
		if dsn == "" {
			return nil, fmt.Errorf("chat backend postgres requires DSN")
		}
		pool, err := newPgPool(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("connect postgres (chat): %w", err)
		}
		return NewPostgresChatStore(pool), nil
	default:
		return nil, fmt.Errorf("unsupported chat backend: %s", backend)
	}
}

func resolveRootBackend(cfg config.DBConfig) string {
	backend := normalizeBackend(cfg.Backend)
	if backend != "" {
		return backend
	}
	if strings.TrimSpace(cfg.DefaultDSN) != "" {
		return "postgres"
	}
	return "sqlite"
}

func resolveBackend(backend, root string) string {
	backend = normalizeBackend(backend)
	if backend != "" {
		return backend
	}
	if root == "" {
		return "sqlite"
	}
	return root
}

func normalizeBackend(backend string) string {
	return strings.ToLower(strings.TrimSpace(backend))
}

func needsSQLite(backends ...string) bool {
	for _, backend := range backends {
		switch normalizeBackend(backend) {
		case "sqlite":
			return true
		case "auto":
			return true
		}
	}
	return false
}

func openSQLite(ctx context.Context, cfg config.SQLiteConfig) (*sql.DB, error) {
	db, err := sqlitep.Open(ctx, sqlitep.Config{
		Path:          cfg.Path,
		BusyTimeoutMs: cfg.BusyTimeoutMs,
		WAL:           cfg.WAL,
		MaxOpenConns:  cfg.MaxOpenConns,
	})
	if err != nil {
		return nil, fmt.Errorf("connect sqlite: %w", err)
	}
	return db, nil
}

func buildSpecialistActivityStore(ctx context.Context, backend, dsn string, sqliteDB *sql.DB) (persistence.SpecialistActivityStore, error) {
	switch backend {
	case "", "memory", "none", "disabled":
		return NewMemorySpecialistActivityStore(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresSpecialistActivityStore(pool), nil
		}
		if sqliteDB != nil {
			return NewSQLiteSpecialistActivityStore(sqliteDB), nil
		}
		return NewMemorySpecialistActivityStore(), nil
	case "sqlite":
		if sqliteDB == nil {
			return nil, fmt.Errorf("specialist activity backend sqlite requires database")
		}
		return NewSQLiteSpecialistActivityStore(sqliteDB), nil
	case "postgres", "pg":
		if dsn == "" {
			return nil, fmt.Errorf("specialist activity backend postgres requires DSN")
		}
		pool, err := newPgPool(ctx, dsn)
		if err != nil {
			return nil, fmt.Errorf("connect postgres (specialist activity): %w", err)
		}
		return NewPostgresSpecialistActivityStore(pool), nil
	default:
		return nil, fmt.Errorf("unsupported specialist activity backend: %s", backend)
	}
}

func initializeDefaultStores(ctx context.Context, m *Manager, cfg config.DBConfig, chatDSN string) error {
	configureDefaultPostgresStores(ctx, m, cfg)

	defaultBackend := resolveDefaultStoreBackend(cfg, m.SQLite != nil)
	if err := initializeMemoryDefaultStores(ctx, m, cfg, defaultBackend); err != nil {
		return err
	}
	if err := initializeWorkflowDefaultStores(ctx, m, cfg, defaultBackend); err != nil {
		return err
	}
	if err := initializePlaygroundDefaultStore(ctx, m, cfg, chatDSN, defaultBackend); err != nil {
		return err
	}
	if err := initializeConfigDefaultStores(ctx, m, cfg, defaultBackend); err != nil {
		return err
	}
	if err := initializeRuntimeDefaultStores(ctx, m, cfg, defaultBackend); err != nil {
		return err
	}
	if err := initializeBeliefDefaultStore(ctx, m, cfg, defaultBackend); err != nil {
		return err
	}
	return initializeArchaeologyDefaultStores(ctx, m, cfg, defaultBackend)
}

func initializeMemoryDefaultStores(ctx context.Context, m *Manager, cfg config.DBConfig, defaultBackend string) error {
	if defaultBackend != "sqlite" {
		return nil
	}
	m.EvolvingMemory = NewSQLiteEvolvingMemoryStoreWithDimensions(m.SQLite, cfg.Vector.Dimensions)
	if store, ok := m.EvolvingMemory.(interface{ Init(context.Context) error }); ok {
		return initStore(ctx, "evolving memory store", store)
	}
	return nil
}

func initializeWorkflowDefaultStores(ctx context.Context, m *Manager, cfg config.DBConfig, defaultBackend string) error {
	if defaultBackend == "sqlite" {
		m.FlowV2 = NewSQLiteFlowV2Store(m.SQLite)
		m.Durable = durable.NewSQLiteStore(m.SQLite)
	} else {
		m.FlowV2 = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresFlowV2Store)
		m.Durable = durable.NewStore(openOptionalPostgresPool(ctx, cfg.DefaultDSN))
	}
	if err := initStore(ctx, "flow v2 store", m.FlowV2); err != nil {
		return err
	}
	return initStore(ctx, "durable store", m.Durable)
}

func initializePlaygroundDefaultStore(ctx context.Context, m *Manager, cfg config.DBConfig, chatDSN string, defaultBackend string) error {
	playgroundDSN := firstNonEmpty(chatDSN, cfg.DefaultDSN)
	if defaultBackend == "sqlite" {
		store, err := NewSQLitePlaygroundStore(ctx, m.SQLite)
		if err != nil {
			return fmt.Errorf("init playground store: %w", err)
		}
		m.Playground = store
		return nil
	}
	if playgroundDSN == "" {
		return nil
	}
	store, err := NewPlaygroundStoreFromDSN(ctx, playgroundDSN)
	if err != nil {
		return fmt.Errorf("init playground store: %w", err)
	}
	m.Playground = store
	return nil
}

func initializeConfigDefaultStores(ctx context.Context, m *Manager, cfg config.DBConfig, defaultBackend string) error {
	var codec secrets.Codec
	if defaultBackend == "sqlite" || strings.TrimSpace(cfg.DefaultDSN) != "" {
		var err error
		codec, err = databaseSecretCodec(nil)
		if err != nil {
			return err
		}
	}
	if defaultBackend == "sqlite" {
		m.Specialists = NewSQLiteSpecialistsStoreWithCodec(m.SQLite, codec)
		m.SpecialistTeams = NewSQLiteSpecialistTeamsStore(m.SQLite)
		m.MCP = NewSQLiteMCPStoreWithCodec(m.SQLite, codec)
		m.Projects = NewSQLiteProjectsStore(m.SQLite)
		m.UserPreferences = NewSQLiteUserPreferencesStore(m.SQLite)
		m.CommandPolicy = NewSQLiteCommandPolicyStore(m.SQLite)
	} else {
		m.Specialists = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, func(pool *pgxpool.Pool) persistence.SpecialistsStore {
			return NewSpecialistsStoreWithCodec(pool, codec)
		})
		m.SpecialistTeams = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewSpecialistTeamsStore)
		m.MCP = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, func(pool *pgxpool.Pool) persistence.MCPStore {
			return NewMCPStoreWithCodec(pool, codec)
		})
		m.Projects = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresProjectsStore)
		m.UserPreferences = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewUserPreferencesStore)
		m.CommandPolicy = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewCommandPolicyStore)
	}
	if err := initStore(ctx, "specialists store", m.Specialists); err != nil {
		return err
	}
	if err := initStore(ctx, "specialist teams store", m.SpecialistTeams); err != nil {
		return err
	}
	if err := initStore(ctx, "mcp store", m.MCP); err != nil {
		return err
	}
	if err := initStore(ctx, "projects store", m.Projects); err != nil {
		return err
	}
	if err := initStore(ctx, "user preferences store", m.UserPreferences); err != nil {
		return err
	}
	return initStore(ctx, "command policy store", m.CommandPolicy)
}

func initializeRuntimeDefaultStores(ctx context.Context, m *Manager, cfg config.DBConfig, defaultBackend string) error {
	if defaultBackend == "sqlite" {
		m.Pulse = NewSQLitePulseStore(m.SQLite)
		m.MatrixMessages = NewSQLiteMatrixMessageStore(m.SQLite)
		m.Transit = NewSQLiteTransitStore(m.SQLite)
	} else {
		m.Pulse = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPulseStore)
		m.MatrixMessages = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewMatrixMessageStore)
		m.Transit = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresTransitStore)
	}
	if err := initStore(ctx, "pulse store", m.Pulse); err != nil {
		return err
	}
	if err := initStore(ctx, "matrix message store", m.MatrixMessages); err != nil {
		return err
	}
	return initStore(ctx, "transit store", m.Transit)
}

func initializeBeliefDefaultStore(ctx context.Context, m *Manager, cfg config.DBConfig, defaultBackend string) error {
	if defaultBackend == "sqlite" {
		m.Belief = NewSQLiteBeliefStore(m.SQLite)
	} else {
		m.Belief = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, func(pool *pgxpool.Pool) belief.Store {
			return NewBeliefStoreWithDimensions(pool, cfg.Vector.Dimensions)
		})
	}
	return initStore(ctx, "belief store", m.Belief)
}

func initializeArchaeologyDefaultStores(ctx context.Context, m *Manager, cfg config.DBConfig, defaultBackend string) error {
	if defaultBackend == "sqlite" {
		m.Decision = NewSQLiteDecisionStore(m.SQLite)
		m.Artifact = NewSQLiteArtifactStore(m.SQLite)
	} else {
		m.Decision = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, func(pool *pgxpool.Pool) decision.Store {
			return NewPostgresDecisionStoreWithDimensions(pool, cfg.Vector.Dimensions)
		})
		m.Artifact = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresArtifactStore)
	}
	if err := initStore(ctx, "decision store", m.Decision); err != nil {
		return err
	}
	return initStore(ctx, "artifact store", m.Artifact)
}

func resolveDefaultStoreBackend(cfg config.DBConfig, sqliteAvailable bool) string {
	backend := resolveRootBackend(cfg)
	switch backend {
	case "sqlite":
		if sqliteAvailable {
			return "sqlite"
		}
	case "auto":
		if strings.TrimSpace(cfg.DefaultDSN) == "" && sqliteAvailable {
			return "sqlite"
		}
	case "postgres", "pg":
		return "postgres"
	}
	return "postgres"
}

func configureDefaultPostgresStores(ctx context.Context, m *Manager, cfg config.DBConfig) {
	backend := resolveRootBackend(cfg)
	if backend == "sqlite" || backend == "memory" || backend == "none" || backend == "disabled" {
		return
	}
	if cfg.DefaultDSN == "" {
		return
	}

	pool := openOptionalPostgresPool(ctx, cfg.DefaultDSN)
	if pool == nil {
		return
	}

	m.EvolvingMemory = NewPostgresEvolvingMemoryStoreWithDimensions(pool, cfg.Vector.Dimensions)
	if store, ok := m.EvolvingMemory.(interface{ Init(context.Context) error }); ok {
		_ = store.Init(ctx)
	}
}

func initStore(ctx context.Context, name string, store interface{ Init(context.Context) error }) error {
	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("init %s: %w", name, err)
	}
	return nil
}

func newStoreWithOptionalPool[T any](ctx context.Context, dsn string, constructor func(*pgxpool.Pool) T) T {
	return constructor(openOptionalPostgresPool(ctx, dsn))
}

func openOptionalPostgresPool(ctx context.Context, dsn string) *pgxpool.Pool {
	if dsn == "" {
		return nil
	}
	pool, err := newPgPool(ctx, dsn)
	if err != nil {
		return nil
	}
	return pool
}

// no-op backends for "none" configuration
type noopSearch struct{}

func (noopSearch) Index(context.Context, string, string, map[string]string) error { return nil }
func (noopSearch) Remove(context.Context, string) error                           { return nil }
func (noopSearch) Search(context.Context, string, int) ([]SearchResult, error)    { return nil, nil }
func (noopSearch) GetByID(context.Context, string) (SearchResult, bool, error) {
	return SearchResult{}, false, nil
}

type noopVector struct{}

func (noopVector) Upsert(context.Context, string, []float32, map[string]string) error { return nil }
func (noopVector) Delete(context.Context, string) error                               { return nil }
func (noopVector) SimilaritySearch(context.Context, []float32, int, map[string]string) ([]VectorResult, error) {
	return nil, nil
}

type noopGraph struct{}

func (noopGraph) UpsertNode(context.Context, string, []string, map[string]any) error { return nil }
func (noopGraph) UpsertEdge(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (noopGraph) Neighbors(context.Context, string, string) ([]string, error) { return nil, nil }
func (noopGraph) GetNode(context.Context, string) (Node, bool)                { return Node{}, false }
func (noopGraph) TypedUpsertEdge(context.Context, string, string, string, string, map[string]any) error {
	return nil
}
func (noopGraph) TypedNeighbors(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}

// helpers
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func newPgPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// Conservative defaults; can be made configurable later
	cfg.MaxConns = 8
	cfg.MinConns = 0
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 5 * time.Minute
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

func ensureVectorExtension(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("postgres pool is nil")
	}
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("enable vector extension: %w", err)
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'vector')`).Scan(&exists); err != nil {
		return fmt.Errorf("check vector type: %w", err)
	}
	if !exists {
		return errors.New("pgvector extension is unavailable")
	}
	return nil
}
