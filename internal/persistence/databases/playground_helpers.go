package databases

import (
	"context"
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
	row := s.pool.QueryRow(ctx, `SELECT payload FROM playground_runs WHERE id=$1 AND user_id=$2`, id, uid)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return playground.Run{}, false, nil
		}
		return playground.Run{}, false, err
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
	return false
}

// Close releases the underlying connection pool.
func (s *PlaygroundStore) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// userIDFromContext returns the authenticated user ID or 0 when not present.
func userIDFromContext(ctx context.Context) int64 {
	if u, ok := auth.CurrentUser(ctx); ok && u != nil {
		return u.ID
	}
	return 0
}
