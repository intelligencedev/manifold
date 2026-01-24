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
		return &memSpecStore{m: map[int64]map[string]persistence.Specialist{}}
	}
	return &pgSpecStore{pool: pool}
}

type memSpecStore struct {
	m map[int64]map[string]persistence.Specialist
}

func (s *memSpecStore) Init(ctx context.Context) error { return nil }

func (s *memSpecStore) List(ctx context.Context, userID int64) ([]persistence.Specialist, error) {
	userMap := s.m[userID]
	if userMap == nil {
		return []persistence.Specialist{}, nil
	}
	out := make([]persistence.Specialist, 0, len(userMap))
	for _, v := range userMap {
		out = append(out, v)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && strings.ToLower(out[j].Name) < strings.ToLower(out[j-1].Name); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

func (s *memSpecStore) GetByName(ctx context.Context, userID int64, name string) (persistence.Specialist, bool, error) {
	if userMap := s.m[userID]; userMap != nil {
		v, ok := userMap[name]
		return v, ok, nil
	}
	return persistence.Specialist{}, false, nil
}

func (s *memSpecStore) Upsert(ctx context.Context, userID int64, sp persistence.Specialist) (persistence.Specialist, error) {
	if strings.TrimSpace(sp.Name) == "" {
		return persistence.Specialist{}, errors.New("name required")
	}
	if s.m[userID] == nil {
		s.m[userID] = map[string]persistence.Specialist{}
	}
	sp.UserID = userID
	s.m[userID][sp.Name] = sp
	return sp, nil
}

func (s *memSpecStore) Delete(ctx context.Context, userID int64, name string) error {
	if s.m[userID] == nil {
		return nil
	}
	delete(s.m[userID], name)
	return nil
}

type pgSpecStore struct {
	pool *pgxpool.Pool
}

func (s *pgSpecStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS specialists (
	id SERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL DEFAULT 0,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	base_url TEXT NOT NULL DEFAULT '',
	api_key TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	summary_context_window_tokens INT NOT NULL DEFAULT 0,
	enable_tools BOOLEAN NOT NULL DEFAULT false,
	paused BOOLEAN NOT NULL DEFAULT false,
	allow_tools JSONB NOT NULL DEFAULT '[]',
	reasoning_effort TEXT NOT NULL DEFAULT '',
	system TEXT NOT NULL DEFAULT '',
	extra_headers JSONB NOT NULL DEFAULT '{}',
	extra_params JSONB NOT NULL DEFAULT '{}',
	provider TEXT NOT NULL DEFAULT ''
);

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS user_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT '';

ALTER TABLE specialists
	ADD COLUMN IF NOT EXISTS summary_context_window_tokens INT NOT NULL DEFAULT 0;

ALTER TABLE specialists
	DROP CONSTRAINT IF EXISTS specialists_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS specialists_user_name_idx ON specialists(user_id, name);
`)
	return err
}

func (s *pgSpecStore) List(ctx context.Context, userID int64) ([]persistence.Specialist, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,provider FROM specialists WHERE user_id=$1 ORDER BY LOWER(name)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []persistence.Specialist
	for rows.Next() {
		var sp persistence.Specialist
		var allow, headers, params []byte
		if err := rows.Scan(&sp.ID, &sp.UserID, &sp.Name, &sp.Description, &sp.BaseURL, &sp.APIKey, &sp.Model, &sp.SummaryContextWindowTokens, &sp.EnableTools, &sp.Paused, &allow, &sp.ReasoningEffort, &sp.System, &headers, &params, &sp.Provider); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(allow, &sp.AllowTools)
		_ = json.Unmarshal(headers, &sp.ExtraHeaders)
		_ = json.Unmarshal(params, &sp.ExtraParams)
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *pgSpecStore) GetByName(ctx context.Context, userID int64, name string) (persistence.Specialist, bool, error) {
	row := s.pool.QueryRow(ctx, `SELECT id,user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,provider FROM specialists WHERE user_id=$1 AND name=$2`, userID, name)
	var sp persistence.Specialist
	var allow, headers, params []byte
	if err := row.Scan(&sp.ID, &sp.UserID, &sp.Name, &sp.Description, &sp.BaseURL, &sp.APIKey, &sp.Model, &sp.SummaryContextWindowTokens, &sp.EnableTools, &sp.Paused, &allow, &sp.ReasoningEffort, &sp.System, &headers, &params, &sp.Provider); err != nil {
		return persistence.Specialist{}, false, nil
	}
	_ = json.Unmarshal(allow, &sp.AllowTools)
	_ = json.Unmarshal(headers, &sp.ExtraHeaders)
	_ = json.Unmarshal(params, &sp.ExtraParams)
	return sp, true, nil
}

func (s *pgSpecStore) Upsert(ctx context.Context, userID int64, sp persistence.Specialist) (persistence.Specialist, error) {
	if strings.TrimSpace(sp.Name) == "" {
		return persistence.Specialist{}, errors.New("name required")
	}
	allow, _ := json.Marshal(sp.AllowTools)
	headers, _ := json.Marshal(sp.ExtraHeaders)
	params, _ := json.Marshal(sp.ExtraParams)
	row := s.pool.QueryRow(ctx, `
INSERT INTO specialists(user_id,name,description,base_url,api_key,model,summary_context_window_tokens,enable_tools,paused,allow_tools,reasoning_effort,system,extra_headers,extra_params,provider)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (user_id, name) DO UPDATE SET description=EXCLUDED.description, base_url=EXCLUDED.base_url, api_key=EXCLUDED.api_key, model=EXCLUDED.model,
	summary_context_window_tokens=EXCLUDED.summary_context_window_tokens, enable_tools=EXCLUDED.enable_tools, paused=EXCLUDED.paused, allow_tools=EXCLUDED.allow_tools,
	reasoning_effort=EXCLUDED.reasoning_effort, system=EXCLUDED.system, extra_headers=EXCLUDED.extra_headers, extra_params=EXCLUDED.extra_params, provider=EXCLUDED.provider
RETURNING id;`, userID, sp.Name, sp.Description, sp.BaseURL, sp.APIKey, sp.Model, sp.SummaryContextWindowTokens, sp.EnableTools, sp.Paused, allow, sp.ReasoningEffort, sp.System, headers, params, sp.Provider)
	if err := row.Scan(&sp.ID); err != nil {
		return persistence.Specialist{}, err
	}
	sp.UserID = userID
	return sp, nil
}

func (s *pgSpecStore) Delete(ctx context.Context, userID int64, name string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM specialists WHERE user_id=$1 AND name=$2`, userID, name)
	return err
}
