package databases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"manifold/internal/persistence"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewSpecialistsStore returns a Postgres-backed store if a pool is provided, otherwise an in-memory store.
func NewSpecialistsStore(pool *pgxpool.Pool) persistence.SpecialistsStore {
	if pool == nil {
		return &memSpecStore{m: map[string]persistence.Specialist{}}
	}
	return &pgSpecStore{pool: pool}
}

type memSpecStore struct {
	m map[string]persistence.Specialist
}

func (s *memSpecStore) Init(ctx context.Context) error { return nil }

func (s *memSpecStore) List(ctx context.Context) ([]persistence.Specialist, error) {
	out := make([]persistence.Specialist, 0, len(s.m))
	for _, v := range s.m {
		out = append(out, v)
	}
	// simple sort by name
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].Name) < strings.ToLower(out[j-1].Name); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (s *memSpecStore) GetByName(ctx context.Context, name string) (persistence.Specialist, bool, error) {
	v, ok := s.m[name]
	return v, ok, nil
}

func (s *memSpecStore) Upsert(ctx context.Context, sp persistence.Specialist) (persistence.Specialist, error) {
	if strings.TrimSpace(sp.Name) == "" {
		return persistence.Specialist{}, errors.New("name required")
	}
	s.m[sp.Name] = sp
	return sp, nil
}

func (s *memSpecStore) Delete(ctx context.Context, name string) error {
	delete(s.m, name)
	return nil
}

type pgSpecStore struct {
	pool *pgxpool.Pool
}

func (s *pgSpecStore) Init(ctx context.Context) error {
	// Minimal schema: specialists table with JSON fields for complex types
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS specialists (
	id SERIAL PRIMARY KEY,
	name TEXT UNIQUE NOT NULL,
	base_url TEXT NOT NULL DEFAULT '',
	api_key TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	enable_tools BOOLEAN NOT NULL DEFAULT false,
	paused BOOLEAN NOT NULL DEFAULT false,
	allow_tools JSONB NOT NULL DEFAULT '[]',
	reasoning_effort TEXT NOT NULL DEFAULT '',
	system TEXT NOT NULL DEFAULT '',
	extra_headers JSONB NOT NULL DEFAULT '{}',
	extra_params JSONB NOT NULL DEFAULT '{}'
);
`)
	return err
}

func (s *pgSpecStore) List(ctx context.Context) ([]persistence.Specialist, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,base_url,api_key,model,enable_tools,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params FROM specialists`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persistence.Specialist
	for rows.Next() {
		var sp persistence.Specialist
		var allow, headers, params []byte
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.BaseURL, &sp.APIKey, &sp.Model, &sp.EnableTools, &sp.Paused, &allow, &sp.ReasoningEffort, &sp.System, &headers, &params); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(allow, &sp.AllowTools)
		_ = json.Unmarshal(headers, &sp.ExtraHeaders)
		_ = json.Unmarshal(params, &sp.ExtraParams)
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *pgSpecStore) GetByName(ctx context.Context, name string) (persistence.Specialist, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT id,name,base_url,api_key,model,enable_tools,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params FROM specialists WHERE name=$1`, name)
	var sp persistence.Specialist
	var allow, headers, params []byte
	if err := row.Scan(&sp.ID, &sp.Name, &sp.BaseURL, &sp.APIKey, &sp.Model, &sp.EnableTools, &sp.Paused, &allow, &sp.ReasoningEffort, &sp.System, &headers, &params); err != nil {
		return persistence.Specialist{}, false, nil
	}
	_ = json.Unmarshal(allow, &sp.AllowTools)
	_ = json.Unmarshal(headers, &sp.ExtraHeaders)
	_ = json.Unmarshal(params, &sp.ExtraParams)
	return sp, true, nil
}

func (s *pgSpecStore) Upsert(ctx context.Context, sp persistence.Specialist) (persistence.Specialist, error) {
	if strings.TrimSpace(sp.Name) == "" {
		return persistence.Specialist{}, errors.New("name required")
	}
	allow, _ := json.Marshal(sp.AllowTools)
	headers, _ := json.Marshal(sp.ExtraHeaders)
	params, _ := json.Marshal(sp.ExtraParams)
	row := s.pool.QueryRow(ctx, `
INSERT INTO specialists(name,base_url,api_key,model,enable_tools,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (name) DO UPDATE SET base_url=EXCLUDED.base_url, api_key=EXCLUDED.api_key, model=EXCLUDED.model,
  enable_tools=EXCLUDED.enable_tools, paused=EXCLUDED.paused, allow_tools=EXCLUDED.allow_tools,
  reasoning_effort=EXCLUDED.reasoning_effort, system=EXCLUDED.system, extra_headers=EXCLUDED.extra_headers, extra_params=EXCLUDED.extra_params
RETURNING id;`, sp.Name, sp.BaseURL, sp.APIKey, sp.Model, sp.EnableTools, sp.Paused, allow, sp.ReasoningEffort, sp.System, headers, params)
	if err := row.Scan(&sp.ID); err != nil {
		return persistence.Specialist{}, err
	}
	return sp, nil
}

func (s *pgSpecStore) Delete(ctx context.Context, name string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM specialists WHERE name=$1`, name)
	return err
}
