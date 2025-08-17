package databases

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgVector struct {
	pool       *pgxpool.Pool
	dimensions int
	metric     string // cosine|l2|ip
}

func NewPostgresVector(pool *pgxpool.Pool, dimensions int, metric string) VectorStore {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`)
	vecType := "vector"
	if dimensions > 0 {
		vecType = fmt.Sprintf("vector(%d)", dimensions)
	}
	_, _ = pool.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS embeddings (
  id TEXT PRIMARY KEY,
  vec %s,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
`, vecType))
	// Index creation left to DBA/tuning; default scan is fine for small data
	return &pgVector{pool: pool, dimensions: dimensions, metric: strings.ToLower(strings.TrimSpace(metric))}
}

func (p *pgVector) Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	vecLit := toVectorLiteral(vector)
	_, err := p.pool.Exec(ctx, `
INSERT INTO embeddings(id, vec, metadata) VALUES($1, $2::vector, $3)
ON CONFLICT (id) DO UPDATE SET vec=EXCLUDED.vec, metadata=EXCLUDED.metadata
`, id, vecLit, metadata)
	return err
}

func (p *pgVector) Delete(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM embeddings WHERE id=$1`, id)
	return err
}

func (p *pgVector) SimilaritySearch(ctx context.Context, vector []float32, k int, filter map[string]string) ([]VectorResult, error) {
	if k <= 0 {
		k = 10
	}
	vecLit := toVectorLiteral(vector)
	op := "<=>" // cosine distance
	scoreExpr := "1 - (vec <=> $1::vector)"
	switch p.metric {
	case "l2", "euclidean":
		op = "<->"
		scoreExpr = "-(vec <-> $1::vector)" // higher is better (less distance)
	case "ip", "dot":
		op = "<#>"
		scoreExpr = "-(vec <#> $1::vector)" // maximize inner product
	}
	args := []any{vecLit, k}
	where := ""
	if len(filter) > 0 {
		where = "WHERE metadata @> $3"
		args = []any{vecLit, k, filter}
	}
	query := fmt.Sprintf(`SELECT id, %s AS score, metadata FROM embeddings %s ORDER BY vec %s $1::vector LIMIT $2`, scoreExpr, where, op)
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]VectorResult, 0, k)
	for rows.Next() {
		var r VectorResult
		var md map[string]string
		if err := rows.Scan(&r.ID, &r.Score, &md); err != nil {
			return nil, err
		}
		r.Metadata = md
		out = append(out, r)
	}
	return out, rows.Err()
}

func toVectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	b := strings.Builder{}
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		// Use %g to avoid trailing zeros; Postgres accepts decimal
		b.WriteString(fmt.Sprintf("%g", x))
	}
	b.WriteByte(']')
	return b.String()
}
