package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

type sqliteGraph struct {
	db *sql.DB
}

func NewSQLiteGraph(db *sql.DB) GraphDB {
	return &sqliteGraph{db: db}
}

func (g *sqliteGraph) Init(ctx context.Context) error {
	if g.db == nil {
		return errors.New("sqlite graph store requires db")
	}
	_, err := g.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS nodes (
	id TEXT PRIMARY KEY,
	labels TEXT NOT NULL DEFAULT '[]',
	props TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS edges (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	source TEXT NOT NULL,
	rel TEXT NOT NULL,
	target TEXT NOT NULL,
	props TEXT NOT NULL DEFAULT '{}',
	UNIQUE(source, rel, target)
);
CREATE INDEX IF NOT EXISTS edges_src_rel ON edges(source, rel);
CREATE INDEX IF NOT EXISTS edges_dst_rel ON edges(target, rel);
CREATE TABLE IF NOT EXISTS magma_events (
	id TEXT PRIMARY KEY,
	tenant TEXT NOT NULL DEFAULT '',
	session TEXT NOT NULL DEFAULT '',
	text TEXT NOT NULL DEFAULT '',
	graphs TEXT NOT NULL DEFAULT '[]',
	semantic_top_k INTEGER NOT NULL DEFAULT 0,
	temporal_attrs TEXT NOT NULL DEFAULT '{}',
	entity_mentions TEXT NOT NULL DEFAULT '[]',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	props TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS magma_entities (
	id TEXT PRIMARY KEY,
	tenant TEXT NOT NULL DEFAULT '',
	type TEXT NOT NULL DEFAULT '',
	canonical_name TEXT NOT NULL DEFAULT '',
	aliases TEXT NOT NULL DEFAULT '[]',
	props TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS magma_edges (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	source TEXT NOT NULL,
	graph_type TEXT NOT NULL CHECK (graph_type IN ('semantic', 'temporal', 'causal', 'entity')),
	rel TEXT NOT NULL,
	target TEXT NOT NULL,
	weight REAL,
	props TEXT NOT NULL DEFAULT '{}',
	UNIQUE(source, graph_type, rel, target)
);
CREATE INDEX IF NOT EXISTS magma_edges_graph_src ON magma_edges(graph_type, source, rel);
CREATE INDEX IF NOT EXISTS magma_edges_graph_dst ON magma_edges(graph_type, target, rel);
`)
	return err
}

func (g *sqliteGraph) UpsertNode(ctx context.Context, id string, labels []string, props map[string]any) error {
	if err := g.Init(ctx); err != nil {
		return err
	}
	if props == nil {
		props = map[string]any{}
	}
	if labels == nil {
		labels = []string{}
	}
	_, err := g.db.ExecContext(ctx, `
INSERT INTO nodes(id, labels, props) VALUES(?, ?, ?)
ON CONFLICT(id) DO UPDATE SET labels=excluded.labels, props=excluded.props
`, id, encodeJSON(labels, "[]"), encodeJSON(props, "{}"))
	if err != nil {
		return err
	}
	return g.upsertMagmaNode(ctx, id, labels, props)
}

func (g *sqliteGraph) UpsertEdge(ctx context.Context, srcID, rel, dstID string, props map[string]any) error {
	if err := g.Init(ctx); err != nil {
		return err
	}
	if props == nil {
		props = map[string]any{}
	}
	_, err := g.db.ExecContext(ctx, `
INSERT INTO edges(source, rel, target, props) VALUES(?, ?, ?, ?)
ON CONFLICT(source, rel, target) DO UPDATE SET props=excluded.props
`, srcID, rel, dstID, encodeJSON(props, "{}"))
	return err
}

func (g *sqliteGraph) TypedUpsertEdge(ctx context.Context, srcID, graphType, rel, dstID string, props map[string]any) error {
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

func (g *sqliteGraph) Neighbors(ctx context.Context, id string, rel string) ([]string, error) {
	if err := g.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := g.db.QueryContext(ctx, `SELECT target FROM edges WHERE source = ? AND rel = ? ORDER BY target`, id, rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func (g *sqliteGraph) TypedNeighbors(ctx context.Context, id, graphType, rel string) ([]string, error) {
	return g.Neighbors(ctx, id, typedRel(graphType, rel))
}

func (g *sqliteGraph) GetNode(ctx context.Context, id string) (Node, bool) {
	if err := g.Init(ctx); err != nil {
		return Node{}, false
	}
	var labelsRaw, propsRaw string
	err := g.db.QueryRowContext(ctx, `SELECT labels, props FROM nodes WHERE id = ?`, id).Scan(&labelsRaw, &propsRaw)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return Node{}, false
	}
	var labels []string
	_ = json.Unmarshal([]byte(labelsRaw), &labels)
	return Node{ID: id, Labels: labels, Props: decodeAnyMap(propsRaw)}, true
}

func (g *sqliteGraph) ListMagmaEvents(ctx context.Context) ([]MagmaEventSummary, error) {
	if err := g.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := g.db.QueryContext(ctx, `SELECT id, tenant, session, created_at FROM magma_events ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []MagmaEventSummary{}
	for rows.Next() {
		var event MagmaEventSummary
		if err := rows.Scan(&event.ID, &event.Tenant, &event.Session, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (g *sqliteGraph) ListMagmaEdges(ctx context.Context) ([]TypedEdge, error) {
	if err := g.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := g.db.QueryContext(ctx, `
SELECT source, graph_type, rel, target, COALESCE(weight, 0), props
FROM magma_edges
ORDER BY source, graph_type, rel, target`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []TypedEdge{}
	for rows.Next() {
		var edge TypedEdge
		var propsRaw string
		if err := rows.Scan(&edge.Source, &edge.GraphType, &edge.Rel, &edge.Target, &edge.Weight, &propsRaw); err != nil {
			return nil, err
		}
		edge.Props = decodeAnyMap(propsRaw)
		out = append(out, edge)
	}
	return out, rows.Err()
}

func (g *sqliteGraph) UpsertMagmaEdgeProps(ctx context.Context, edge TypedEdge) error {
	return g.TypedUpsertEdge(ctx, edge.Source, edge.GraphType, edge.Rel, edge.Target, edge.Props)
}

func (g *sqliteGraph) DeleteMagmaEdge(ctx context.Context, srcID, graphType, rel, dstID string) error {
	if err := g.Init(ctx); err != nil {
		return err
	}
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	if _, err := tx.ExecContext(ctx, `DELETE FROM magma_edges WHERE source = ? AND graph_type = ? AND rel = ? AND target = ?`, srcID, graphType, rel, dstID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM edges WHERE source = ? AND rel = ? AND target = ?`, srcID, typedRel(graphType, rel), dstID); err != nil {
		return err
	}
	return tx.Commit()
}

func (g *sqliteGraph) DeleteMagmaEvent(ctx context.Context, id string) error {
	if err := g.Init(ctx); err != nil {
		return err
	}
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackQuietly(tx)
	for _, stmt := range []string{
		`DELETE FROM magma_edges WHERE source = ? OR target = ?`,
		`DELETE FROM edges WHERE source = ? OR target = ?`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, id, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM magma_events WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (g *sqliteGraph) upsertMagmaNode(ctx context.Context, id string, labels []string, props map[string]any) error {
	switch {
	case slices.Contains(labels, "MagmaEvent"):
		createdAt := time.Now().UTC()
		if raw, ok := props["created_at"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				createdAt = parsed
			}
		}
		_, err := g.db.ExecContext(ctx, `
INSERT INTO magma_events(id, tenant, session, text, graphs, semantic_top_k, temporal_attrs, entity_mentions, created_at, props)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	tenant=excluded.tenant,
	session=excluded.session,
	text=excluded.text,
	graphs=excluded.graphs,
	semantic_top_k=excluded.semantic_top_k,
	temporal_attrs=excluded.temporal_attrs,
	entity_mentions=excluded.entity_mentions,
	created_at=excluded.created_at,
	props=excluded.props
`, id, stringProp(props, "tenant"), stringProp(props, "session"), stringProp(props, "text"), encodeJSON(jsonProp(props, "graphs", "[]"), "[]"), intProp(props, "semantic_top_k"), encodeJSON(jsonProp(props, "temporal_attrs", "{}"), "{}"), encodeJSON(jsonProp(props, "entity_mentions", "[]"), "[]"), createdAt, encodeJSON(props, "{}"))
		return err
	case slices.Contains(labels, "MagmaEntity"):
		_, err := g.db.ExecContext(ctx, `
INSERT INTO magma_entities(id, tenant, type, canonical_name, props)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	tenant=excluded.tenant,
	type=excluded.type,
	canonical_name=excluded.canonical_name,
	props=excluded.props
`, id, stringProp(props, "tenant"), stringProp(props, "type"), stringProp(props, "canonical_name"), encodeJSON(props, "{}"))
		return err
	default:
		return nil
	}
}

func (g *sqliteGraph) upsertMagmaEdge(ctx context.Context, srcID, graphType, rel, dstID string, props map[string]any) error {
	if !slices.Contains([]string{"semantic", "temporal", "causal", "entity"}, graphType) {
		return nil
	}
	if props == nil {
		props = map[string]any{}
	}
	_, err := g.db.ExecContext(ctx, `
INSERT INTO magma_edges(source, graph_type, rel, target, weight, props)
VALUES(?, ?, ?, ?, ?, ?)
ON CONFLICT(source, graph_type, rel, target) DO UPDATE SET
	weight=excluded.weight,
	props=excluded.props
`, srcID, graphType, rel, dstID, floatProp(props, "weight"), encodeJSON(props, "{}"))
	return err
}

func encodeJSON(value any, fallback string) string {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 {
		return fallback
	}
	return string(raw)
}

func decodeAnyMap(raw string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}
