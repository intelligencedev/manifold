package databases

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"manifold/internal/auth"
	"manifold/internal/playground"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"
)

func cloneMetrics(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	maps.Copy(out, in)
	return out
}

func (s *PlaygroundStore) getRun(ctx context.Context, id string, uid int64) (playground.Run, bool, error) {
	payload, ok, err := s.queryOnePayload(ctx, `SELECT payload FROM playground_runs WHERE id=$1 AND user_id=$2`, id, uid)
	if err != nil || !ok {
		return playground.Run{}, ok, err
	}
	var run playground.Run
	if err := json.Unmarshal(payload, &run); err != nil {
		return playground.Run{}, false, err
	}
	return run, true, nil
}

func isPGConstraint(err error) bool {
	// Any constraint violation surfaces via pgx's pgconn.PgError with Code starting with "23".
	type causer interface{ SQLState() string }
	var c causer
	if errors.As(err, &c) {
		if strings.HasPrefix(c.SQLState(), "23") {
			return true
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return true
	}
	return false
}

// Close releases the underlying connection pool.
func (s *PlaygroundStore) Close() {
	if s == nil {
		return
	}
	if s.pool != nil {
		s.pool.Close()
	}
	if s.db != nil {
		_ = s.db.Close()
	}
}

// userIDFromContext returns the authenticated user ID or 0 when not present.
func userIDFromContext(ctx context.Context) int64 {
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		return u.ID
	}
	return 0
}

func (s *PlaygroundStore) exec(ctx context.Context, query string, args ...any) (int64, error) {
	if s.db != nil {
		res, err := s.db.ExecContext(ctx, sqliteQuestionPlaceholders(query), args...)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PlaygroundStore) queryOnePayload(ctx context.Context, query string, args ...any) ([]byte, bool, error) {
	var payload []byte
	if s.db != nil {
		err := s.db.QueryRowContext(ctx, sqliteQuestionPlaceholders(query), args...).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		return payload, true, nil
	}
	row := s.pool.QueryRow(ctx, query, args...)
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return payload, true, nil
}

func (s *PlaygroundStore) queryPayloads(ctx context.Context, query string, args ...any) ([][]byte, error) {
	if s.db != nil {
		rows, err := s.db.QueryContext(ctx, sqliteQuestionPlaceholders(query), args...)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		return scanPlaygroundPayloadRows(rows)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlaygroundPayloadRows(rows)
}

type playgroundPayloadRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanPlaygroundPayloadRows(rows playgroundPayloadRows) ([][]byte, error) {
	var payloads [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	return payloads, rows.Err()
}

func sqliteQuestionPlaceholders(query string) string {
	var builder strings.Builder
	builder.Grow(len(query))
	for i := 0; i < len(query); i++ {
		if query[i] != '$' {
			builder.WriteByte(query[i])
			continue
		}
		j := i + 1
		for j < len(query) && query[j] >= '0' && query[j] <= '9' {
			j++
		}
		if j == i+1 {
			builder.WriteByte(query[i])
			continue
		}
		builder.WriteByte('?')
		i = j - 1
	}
	return builder.String()
}
