package databases

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgGraph struct{ pool *pgxpool.Pool }

func NewPostgresGraph(pool *pgxpool.Pool) GraphDB {
	ctx := context.Background()
	// Extensions best-effort; may require superuser
	_, _ = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS postgis`)
	_, _ = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgrouting`)
	_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  labels TEXT[] NOT NULL DEFAULT '{}',
  props JSONB NOT NULL DEFAULT '{}'::jsonb
);
`)
	_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS edges (
  id BIGSERIAL PRIMARY KEY,
  source TEXT NOT NULL,
  rel TEXT NOT NULL,
  target TEXT NOT NULL,
  props JSONB NOT NULL DEFAULT '{}'::jsonb
);
`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS edges_src_rel ON edges(source, rel)`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS edges_dst_rel ON edges(target, rel)`)
	return &pgGraph{pool: pool}
}

func (g *pgGraph) UpsertNode(ctx context.Context, id string, labels []string, props map[string]any) error {
	// Ensure we never pass SQL NULL for the JSONB `props` column. If callers
	// provide nil, use an empty JSON object so the DB's NOT NULL constraint is
	// satisfied and default behavior is consistent.
	if props == nil {
		props = map[string]any{}
	}
	_, err := g.pool.Exec(ctx, `
INSERT INTO nodes(id, labels, props) VALUES($1,$2,$3)
ON CONFLICT (id) DO UPDATE SET labels=EXCLUDED.labels, props=EXCLUDED.props
`, id, labels, props)
	return err
}

func (g *pgGraph) UpsertEdge(ctx context.Context, srcID, rel, dstID string, props map[string]any) error {
	// Same protection for edges.props
	if props == nil {
		props = map[string]any{}
	}
	_, err := g.pool.Exec(ctx, `
INSERT INTO edges(source, rel, target, props) VALUES($1,$2,$3,$4)
ON CONFLICT DO NOTHING
`, srcID, rel, dstID, props)
	return err
}

func (g *pgGraph) Neighbors(ctx context.Context, id string, rel string) ([]string, error) {
	rows, err := g.pool.Query(ctx, `SELECT target FROM edges WHERE source=$1 AND rel=$2 ORDER BY target`, id, rel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{} // return empty slice rather than nil so JSON encodes as []
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (g *pgGraph) GetNode(ctx context.Context, id string) (Node, bool) {
	row := g.pool.QueryRow(ctx, `SELECT labels, props FROM nodes WHERE id=$1`, id)
	var labels []string
	var props map[string]any
	if err := row.Scan(&labels, &props); err != nil {
		return Node{}, false
	}
	return Node{ID: id, Labels: labels, Props: props}, true
}
