package databases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	persist "manifold/internal/persistence"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresWarppStore returns a Postgres-backed WARPP workflow store.
func NewPostgresWarppStore(pool *pgxpool.Pool) persist.WarppWorkflowStore {
	if pool == nil {
		return &memWarppStore{m: map[int64]map[string]persist.WarppWorkflow{}}
	}
	return &pgWarppStore{pool: pool}
}

// In-memory fallback for tests/dev when no DB configured.
type memWarppStore struct {
	m map[int64]map[string]persist.WarppWorkflow
}

func (s *memWarppStore) Init(context.Context) error { return nil }
func (s *memWarppStore) List(ctx context.Context, userID int64) ([]any, error) { // deprecated
	wfs, _ := s.ListWorkflows(ctx, userID)
	out := make([]any, len(wfs))
	for i, v := range wfs {
		out[i] = v
	}
	return out, nil
}
func (s *memWarppStore) ListWorkflows(_ context.Context, userID int64) ([]persist.WarppWorkflow, error) {
	userMap := s.m[userID]
	if userMap == nil {
		return []persist.WarppWorkflow{}, nil
	}
	out := make([]persist.WarppWorkflow, 0, len(userMap))
	for _, v := range userMap {
		out = append(out, v)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].Intent) < strings.ToLower(out[j-1].Intent); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}
func (s *memWarppStore) GetByIntent(ctx context.Context, userID int64, intent string) (persist.WarppWorkflow, bool, error) {
	if userMap := s.m[userID]; userMap != nil {
		v, ok := userMap[intent]
		return v, ok, nil
	}
	return persist.WarppWorkflow{}, false, nil
}
func (s *memWarppStore) Upsert(ctx context.Context, userID int64, wf persist.WarppWorkflow) (persist.WarppWorkflow, error) {
	if strings.TrimSpace(wf.Intent) == "" {
		return persist.WarppWorkflow{}, errors.New("intent required")
	}
	if s.m[userID] == nil {
		s.m[userID] = map[string]persist.WarppWorkflow{}
	}
	wf.UserID = userID
	s.m[userID][wf.Intent] = wf
	return wf, nil
}
func (s *memWarppStore) Delete(ctx context.Context, userID int64, intent string) error {
	if s.m[userID] == nil {
		return nil
	}
	delete(s.m[userID], intent)
	return nil
}

// Postgres-backed implementation
type pgWarppStore struct{ pool *pgxpool.Pool }

func (s *pgWarppStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS warpp_workflows (
  id SERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL DEFAULT 0,
  intent TEXT NOT NULL,
  doc JSONB NOT NULL,
  description TEXT GENERATED ALWAYS AS (coalesce((doc->>'description'),'') ) STORED
);

ALTER TABLE warpp_workflows
	ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE warpp_workflows
	DROP CONSTRAINT IF EXISTS warpp_workflows_intent_key;

CREATE UNIQUE INDEX IF NOT EXISTS warpp_workflows_user_intent_idx ON warpp_workflows(user_id, intent);
`)
	return err
}

func (s *pgWarppStore) List(ctx context.Context, userID int64) ([]any, error) { // deprecated
	wfs, err := s.ListWorkflows(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(wfs))
	for i, w := range wfs {
		out[i] = w
	}
	return out, nil
}

func (s *pgWarppStore) ListWorkflows(ctx context.Context, userID int64) ([]persist.WarppWorkflow, error) {
	rows, err := s.pool.Query(ctx, `SELECT doc FROM warpp_workflows WHERE user_id=$1 ORDER BY intent`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persist.WarppWorkflow
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var wf persist.WarppWorkflow
		if err := json.Unmarshal(b, &wf); err != nil {
			return nil, err
		}
		wf.UserID = userID
		out = append(out, wf)
	}
	return out, rows.Err()
}

func (s *pgWarppStore) GetByIntent(ctx context.Context, userID int64, intent string) (persist.WarppWorkflow, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT doc FROM warpp_workflows WHERE user_id=$1 AND intent=$2`, userID, intent)
	var b []byte
	if err := row.Scan(&b); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return persist.WarppWorkflow{}, false, nil
		}
		return persist.WarppWorkflow{}, false, err
	}
	var wf persist.WarppWorkflow
	if err := json.Unmarshal(b, &wf); err != nil {
		return persist.WarppWorkflow{}, false, err
	}
	wf.UserID = userID
	return wf, true, nil
}

func (s *pgWarppStore) Upsert(ctx context.Context, userID int64, wf persist.WarppWorkflow) (persist.WarppWorkflow, error) {
	if strings.TrimSpace(wf.Intent) == "" {
		return persist.WarppWorkflow{}, errors.New("intent required")
	}
	b, _ := json.Marshal(wf)
	_, err := s.pool.Exec(ctx, `
INSERT INTO warpp_workflows(user_id, intent, doc) VALUES($1,$2,$3)
ON CONFLICT (user_id, intent) DO UPDATE SET doc=EXCLUDED.doc
`, userID, wf.Intent, b)
	if err != nil {
		return persist.WarppWorkflow{}, err
	}
	wf.UserID = userID
	return wf, nil
}

func (s *pgWarppStore) Delete(ctx context.Context, userID int64, intent string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM warpp_workflows WHERE user_id=$1 AND intent=$2`, userID, intent)
	return err
}
