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
        return &memWarppStore{m: map[string]persist.WarppWorkflow{}}
    }
    return &pgWarppStore{pool: pool}
}

// In-memory fallback for tests/dev when no DB configured.
type memWarppStore struct{ m map[string]persist.WarppWorkflow }

func (s *memWarppStore) Init(context.Context) error { return nil }
func (s *memWarppStore) List(context.Context) ([]any, error) { // deprecated
    out := make([]any, 0, len(s.m))
    for _, v := range s.m { out = append(out, v) }
    return out, nil
}
func (s *memWarppStore) ListWorkflows(context.Context) ([]persist.WarppWorkflow, error) {
    out := make([]persist.WarppWorkflow, 0, len(s.m))
    for _, v := range s.m { out = append(out, v) }
    // simple stable-ish order
    for i:=1;i<len(out);i++{ for j:=i; j>0 && strings.ToLower(out[j].Intent) < strings.ToLower(out[j-1].Intent); j-- { out[j],out[j-1]=out[j-1],out[j] } }
    return out, nil
}
func (s *memWarppStore) Get(ctx context.Context, intent string) (persist.WarppWorkflow, bool, error) {
    v, ok := s.m[intent]
    return v, ok, nil
}
func (s *memWarppStore) Upsert(ctx context.Context, wf persist.WarppWorkflow) (persist.WarppWorkflow, error) {
    if strings.TrimSpace(wf.Intent) == "" { return persist.WarppWorkflow{}, errors.New("intent required") }
    s.m[wf.Intent] = wf
    return wf, nil
}
func (s *memWarppStore) Delete(ctx context.Context, intent string) error {
    delete(s.m, intent); return nil
}

// Postgres-backed implementation
type pgWarppStore struct{ pool *pgxpool.Pool }

func (s *pgWarppStore) Init(ctx context.Context) error {
    _, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS warpp_workflows (
  id SERIAL PRIMARY KEY,
  intent TEXT UNIQUE NOT NULL,
  doc JSONB NOT NULL,
  description TEXT GENERATED ALWAYS AS (coalesce((doc->>'description'),'') ) STORED
);
`)
    return err
}

func (s *pgWarppStore) List(ctx context.Context) ([]any, error) { // deprecated
    wfs, err := s.ListWorkflows(ctx)
    if err != nil { return nil, err }
    out := make([]any, len(wfs))
    for i, w := range wfs { out[i] = w }
    return out, nil
}

func (s *pgWarppStore) ListWorkflows(ctx context.Context) ([]persist.WarppWorkflow, error) {
    rows, err := s.pool.Query(ctx, `SELECT doc FROM warpp_workflows ORDER BY intent`)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []persist.WarppWorkflow
    for rows.Next() {
        var b []byte
        if err := rows.Scan(&b); err != nil { return nil, err }
        var wf persist.WarppWorkflow
        if err := json.Unmarshal(b, &wf); err != nil { return nil, err }
        out = append(out, wf)
    }
    return out, rows.Err()
}

func (s *pgWarppStore) Get(ctx context.Context, intent string) (persist.WarppWorkflow, bool, error) {
    row := s.pool.QueryRow(ctx, `SELECT doc FROM warpp_workflows WHERE intent=$1`, intent)
    var b []byte
    if err := row.Scan(&b); err != nil {
        if errors.Is(err, pgx.ErrNoRows) { return persist.WarppWorkflow{}, false, nil }
        return persist.WarppWorkflow{}, false, err
    }
    var wf persist.WarppWorkflow
    if err := json.Unmarshal(b, &wf); err != nil { return persist.WarppWorkflow{}, false, err }
    return wf, true, nil
}

func (s *pgWarppStore) Upsert(ctx context.Context, wf persist.WarppWorkflow) (persist.WarppWorkflow, error) {
    if strings.TrimSpace(wf.Intent) == "" { return persist.WarppWorkflow{}, errors.New("intent required") }
    b, _ := json.Marshal(wf)
    _, err := s.pool.Exec(ctx, `
INSERT INTO warpp_workflows(intent, doc) VALUES($1,$2)
ON CONFLICT (intent) DO UPDATE SET doc=EXCLUDED.doc
`, wf.Intent, b)
    if err != nil { return persist.WarppWorkflow{}, err }
    return wf, nil
}

func (s *pgWarppStore) Delete(ctx context.Context, intent string) error {
    _, err := s.pool.Exec(ctx, `DELETE FROM warpp_workflows WHERE intent=$1`, intent)
    return err
}

