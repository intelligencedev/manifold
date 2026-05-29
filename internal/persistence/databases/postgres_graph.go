package databases

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

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
	_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS magma_events (
  id TEXT PRIMARY KEY,
  tenant TEXT NOT NULL DEFAULT '',
  session TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  graphs JSONB NOT NULL DEFAULT '[]'::jsonb,
  semantic_top_k INT NOT NULL DEFAULT 0,
  temporal_attrs JSONB NOT NULL DEFAULT '{}'::jsonb,
  entity_mentions JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  props JSONB NOT NULL DEFAULT '{}'::jsonb
);
`)
	_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS magma_entities (
  id TEXT PRIMARY KEY,
  tenant TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',
  canonical_name TEXT NOT NULL DEFAULT '',
  aliases TEXT[] NOT NULL DEFAULT '{}',
  props JSONB NOT NULL DEFAULT '{}'::jsonb
);
`)
	_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS magma_edges (
  id BIGSERIAL PRIMARY KEY,
  source TEXT NOT NULL,
  graph_type TEXT NOT NULL CHECK (graph_type IN ('semantic', 'temporal', 'causal', 'entity')),
  rel TEXT NOT NULL,
  target TEXT NOT NULL,
  weight FLOAT8,
  props JSONB NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE(source, graph_type, rel, target)
);
`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS magma_edges_graph_src ON magma_edges(graph_type, source, rel)`)
	_, _ = pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS magma_edges_graph_dst ON magma_edges(graph_type, target, rel)`)
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
	if err != nil {
		return err
	}
	return g.upsertMagmaNode(ctx, id, labels, props)
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

func (g *pgGraph) TypedUpsertEdge(ctx context.Context, srcID, graphType, rel, dstID string, props map[string]any) error {
	cp := make(map[string]any, len(props)+1)
	for k, v := range props {
		cp[k] = v
	}
	cp["graph_type"] = graphType
	if err := g.UpsertEdge(ctx, srcID, typedRel(graphType, rel), dstID, cp); err != nil {
		return err
	}
	return g.upsertMagmaEdge(ctx, srcID, graphType, rel, dstID, props)
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

func (g *pgGraph) TypedNeighbors(ctx context.Context, id, graphType, rel string) ([]string, error) {
	return g.Neighbors(ctx, id, typedRel(graphType, rel))
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

func (g *pgGraph) upsertMagmaNode(ctx context.Context, id string, labels []string, props map[string]any) error {
	switch {
	case slices.Contains(labels, "MagmaEvent"):
		createdAt := time.Now().UTC()
		if raw, ok := props["created_at"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				createdAt = parsed
			}
		}
		_, err := g.pool.Exec(ctx, `
INSERT INTO magma_events(id, tenant, session, text, graphs, semantic_top_k, temporal_attrs, entity_mentions, created_at, props)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (id) DO UPDATE SET
  tenant=EXCLUDED.tenant,
  session=EXCLUDED.session,
  text=EXCLUDED.text,
  graphs=EXCLUDED.graphs,
  semantic_top_k=EXCLUDED.semantic_top_k,
  temporal_attrs=EXCLUDED.temporal_attrs,
  entity_mentions=EXCLUDED.entity_mentions,
  created_at=EXCLUDED.created_at,
  props=EXCLUDED.props
`, id, stringProp(props, "tenant"), stringProp(props, "session"), stringProp(props, "text"), jsonProp(props, "graphs", "[]"), intProp(props, "semantic_top_k"), jsonProp(props, "temporal_attrs", "{}"), jsonProp(props, "entity_mentions", "[]"), createdAt, props)
		return err
	case slices.Contains(labels, "MagmaEntity"):
		_, err := g.pool.Exec(ctx, `
INSERT INTO magma_entities(id, tenant, type, canonical_name, props)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT (id) DO UPDATE SET
  tenant=EXCLUDED.tenant,
  type=EXCLUDED.type,
  canonical_name=EXCLUDED.canonical_name,
  props=EXCLUDED.props
`, id, stringProp(props, "tenant"), stringProp(props, "type"), stringProp(props, "canonical_name"), props)
		return err
	default:
		return nil
	}
}

func (g *pgGraph) upsertMagmaEdge(ctx context.Context, srcID, graphType, rel, dstID string, props map[string]any) error {
	if !slices.Contains([]string{"semantic", "temporal", "causal", "entity"}, graphType) {
		return nil
	}
	if props == nil {
		props = map[string]any{}
	}
	_, err := g.pool.Exec(ctx, `
INSERT INTO magma_edges(source, graph_type, rel, target, weight, props)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT (source, graph_type, rel, target) DO UPDATE SET
  weight=EXCLUDED.weight,
  props=EXCLUDED.props
`, srcID, graphType, rel, dstID, floatProp(props, "weight"), props)
	return err
}

func stringProp(props map[string]any, key string) string {
	if value, ok := props[key].(string); ok {
		return value
	}
	return ""
}

func jsonProp(props map[string]any, key, fallback string) any {
	raw, ok := props[key]
	if !ok || raw == nil {
		return json.RawMessage(fallback)
	}
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return json.RawMessage(fallback)
		}
		return json.RawMessage(value)
	case []byte:
		if len(value) == 0 {
			return json.RawMessage(fallback)
		}
		return json.RawMessage(value)
	default:
		return raw
	}
}

func intProp(props map[string]any, key string) int {
	switch value := props[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		i, _ := value.Int64()
		return int(i)
	default:
		return 0
	}
}

func floatProp(props map[string]any, key string) *float64 {
	switch value := props[key].(type) {
	case float64:
		return &value
	case float32:
		out := float64(value)
		return &out
	case int:
		out := float64(value)
		return &out
	case int64:
		out := float64(value)
		return &out
	case json.Number:
		if out, err := value.Float64(); err == nil {
			return &out
		}
	}
	return nil
}
