package databases

import (
	"context"
	"fmt"
	"time"

	"manifold/internal/config"
	"manifold/internal/persistence"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewManager constructs database backends based on configuration.
// Supported backends: memory, none, auto, postgres.
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

	m.Search, err = buildSearchStore(ctx, cfg.Search.Backend, searchDSN)
	if err != nil {
		return Manager{}, err
	}

	m.Vector, err = buildVectorStore(ctx, cfg.Vector, vectorDSN)
	if err != nil {
		return Manager{}, err
	}

	m.Graph, err = buildGraphStore(ctx, cfg.Graph.Backend, graphDSN)
	if err != nil {
		return Manager{}, err
	}

	m.Chat, err = buildChatStore(ctx, cfg.Chat.Backend, chatDSN)
	if err != nil {
		return Manager{}, err
	}

	if m.Chat == nil {
		m.Chat = newMemoryChatStore()
	}
	if err := initStore(ctx, "chat store", m.Chat); err != nil {
		return Manager{}, err
	}

	if err := initializeDefaultStores(ctx, &m, cfg, chatDSN); err != nil {
		return Manager{}, err
	}

	return m, nil
}

func buildSearchStore(ctx context.Context, backend, dsn string) (FullTextSearch, error) {
	switch backend {
	case "", "memory":
		return NewMemorySearch(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresSearch(pool), nil
		}
		return NewMemorySearch(), nil
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

func buildVectorStore(ctx context.Context, cfg config.VectorConfig, dsn string) (VectorStore, error) {
	switch cfg.Backend {
	case "", "memory":
		return NewMemoryVector(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresVector(pool, cfg.Dimensions, cfg.Metric), nil
		}
		return NewMemoryVector(), nil
	case "postgres", "pgvector", "pg":
		if dsn == "" {
			return nil, fmt.Errorf("vector backend postgres requires DSN")
		}
		pool, err := newPgPool(ctx, dsn)
		if err != nil {
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

func buildGraphStore(ctx context.Context, backend, dsn string) (GraphDB, error) {
	switch backend {
	case "", "memory":
		return NewMemoryGraph(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresGraph(pool), nil
		}
		return NewMemoryGraph(), nil
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

func buildChatStore(ctx context.Context, backend, dsn string) (persistence.ChatStore, error) {
	switch backend {
	case "", "memory", "none", "disabled":
		return newMemoryChatStore(), nil
	case "auto":
		if pool := openOptionalPostgresPool(ctx, dsn); pool != nil {
			return NewPostgresChatStore(pool), nil
		}
		return newMemoryChatStore(), nil
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

func initializeDefaultStores(ctx context.Context, m *Manager, cfg config.DBConfig, chatDSN string) error {
	configureDefaultPostgresStores(ctx, m, cfg.DefaultDSN)

	m.FlowV2 = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresFlowV2Store)
	if err := initStore(ctx, "flow v2 store", m.FlowV2); err != nil {
		return err
	}

	playgroundDSN := firstNonEmpty(chatDSN, cfg.DefaultDSN)
	if playgroundDSN != "" {
		store, err := NewPlaygroundStoreFromDSN(ctx, playgroundDSN)
		if err != nil {
			return fmt.Errorf("init playground store: %w", err)
		}
		m.Playground = store
	}

	m.MCP = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewMCPStore)
	if err := initStore(ctx, "mcp store", m.MCP); err != nil {
		return err
	}

	m.Projects = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresProjectsStore)
	if err := initStore(ctx, "projects store", m.Projects); err != nil {
		return err
	}

	m.UserPreferences = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewUserPreferencesStore)
	if err := initStore(ctx, "user preferences store", m.UserPreferences); err != nil {
		return err
	}

	m.Pulse = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPulseStore)
	if err := initStore(ctx, "pulse store", m.Pulse); err != nil {
		return err
	}

	m.Transit = newStoreWithOptionalPool(ctx, cfg.DefaultDSN, NewPostgresTransitStore)
	if err := initStore(ctx, "transit store", m.Transit); err != nil {
		return err
	}

	return nil
}

func configureDefaultPostgresStores(ctx context.Context, m *Manager, defaultDSN string) {
	if defaultDSN == "" {
		return
	}

	pool := openOptionalPostgresPool(ctx, defaultDSN)
	if pool == nil {
		return
	}

	m.EvolvingMemory = NewPostgresEvolvingMemoryStore(pool)
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
