package databases

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"singularityio/internal/config"
)

// NewManager constructs database backends based on configuration.
// Supported backends: memory, none, auto, postgres.
func NewManager(ctx context.Context, cfg config.DBConfig) (Manager, error) {
	var m Manager
	// Resolve DSNs with default fallback
	searchDSN := firstNonEmpty(cfg.Search.DSN, cfg.DefaultDSN)
	vectorDSN := firstNonEmpty(cfg.Vector.DSN, cfg.DefaultDSN)
	graphDSN := firstNonEmpty(cfg.Graph.DSN, cfg.DefaultDSN)

	// Full-text search
	switch cfg.Search.Backend {
	case "", "memory":
		m.Search = NewMemorySearch()
	case "auto":
		if searchDSN != "" {
			if p, err := newPgPool(ctx, searchDSN); err == nil {
				m.Search = NewPostgresSearch(p)
			} else {
				m.Search = NewMemorySearch()
			}
		} else {
			m.Search = NewMemorySearch()
		}
	case "postgres", "pg":
		if searchDSN == "" {
			return Manager{}, fmt.Errorf("search backend postgres requires DSN")
		}
		p, err := newPgPool(ctx, searchDSN)
		if err != nil {
			return Manager{}, fmt.Errorf("connect postgres (search): %w", err)
		}
		m.Search = NewPostgresSearch(p)
	case "none", "disabled":
		m.Search = noopSearch{}
	default:
		return Manager{}, fmt.Errorf("unsupported search backend: %s", cfg.Search.Backend)
	}
	// Vector store
	switch cfg.Vector.Backend {
	case "", "memory":
		m.Vector = NewMemoryVector()
	case "auto":
		if vectorDSN != "" {
			if p, err := newPgPool(ctx, vectorDSN); err == nil {
				m.Vector = NewPostgresVector(p, cfg.Vector.Dimensions, cfg.Vector.Metric)
			} else {
				m.Vector = NewMemoryVector()
			}
		} else {
			m.Vector = NewMemoryVector()
		}
	case "postgres", "pgvector", "pg":
		if vectorDSN == "" {
			return Manager{}, fmt.Errorf("vector backend postgres requires DSN")
		}
		p, err := newPgPool(ctx, vectorDSN)
		if err != nil {
			return Manager{}, fmt.Errorf("connect postgres (vector): %w", err)
		}
		m.Vector = NewPostgresVector(p, cfg.Vector.Dimensions, cfg.Vector.Metric)
	case "none", "disabled":
		m.Vector = noopVector{}
	default:
		return Manager{}, fmt.Errorf("unsupported vector backend: %s", cfg.Vector.Backend)
	}
	// Graph DB
	switch cfg.Graph.Backend {
	case "", "memory":
		m.Graph = NewMemoryGraph()
	case "auto":
		if graphDSN != "" {
			if p, err := newPgPool(ctx, graphDSN); err == nil {
				m.Graph = NewPostgresGraph(p)
			} else {
				m.Graph = NewMemoryGraph()
			}
		} else {
			m.Graph = NewMemoryGraph()
		}
	case "postgres", "pg":
		if graphDSN == "" {
			return Manager{}, fmt.Errorf("graph backend postgres requires DSN")
		}
		p, err := newPgPool(ctx, graphDSN)
		if err != nil {
			return Manager{}, fmt.Errorf("connect postgres (graph): %w", err)
		}
		m.Graph = NewPostgresGraph(p)
	case "none", "disabled":
		m.Graph = noopGraph{}
	default:
		return Manager{}, fmt.Errorf("unsupported graph backend: %s", cfg.Graph.Backend)
	}
	return m, nil
}

// no-op backends for "none" configuration
type noopSearch struct{}

func (noopSearch) Index(context.Context, string, string, map[string]string) error { return nil }
func (noopSearch) Remove(context.Context, string) error                           { return nil }
func (noopSearch) Search(context.Context, string, int) ([]SearchResult, error)    { return nil, nil }

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
