package trust

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Budget struct {
	Name       string    `json:"name"`
	Quota      int       `json:"quota"`
	Spent      int       `json:"spent"`
	Unlimited  bool      `json:"unlimited"`
	UpdatedAt  time.Time `json:"updated_at"`
	RefilledAt time.Time `json:"refilled_at"`
}

type Store interface {
	Init(ctx context.Context) error
	List(ctx context.Context) ([]Budget, error)
	Get(ctx context.Context, name string) (Budget, bool, error)
	Upsert(ctx context.Context, budget Budget) (Budget, error)
	Spend(ctx context.Context, name string, delta int) (Budget, error)
	Refill(ctx context.Context, name string, quota int) (Budget, error)
}

func NewStore(pool *pgxpool.Pool) Store {
	if pool == nil {
		return &memStore{budgets: map[string]Budget{}}
	}
	return &pgStore{pool: pool}
}

type memStore struct {
	mu      sync.RWMutex
	budgets map[string]Budget
}

func (s *memStore) Init(context.Context) error { return nil }

func (s *memStore) List(context.Context) ([]Budget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Budget, 0, len(s.budgets))
	for _, budget := range s.budgets {
		out = append(out, budget)
	}
	return out, nil
}

func (s *memStore) Get(_ context.Context, name string) (Budget, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	budget, ok := s.budgets[name]
	return budget, ok, nil
}

func (s *memStore) Upsert(_ context.Context, budget Budget) (Budget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if budget.UpdatedAt.IsZero() {
		budget.UpdatedAt = time.Now().UTC()
	}
	s.budgets[budget.Name] = budget
	return budget, nil
}

func (s *memStore) Spend(_ context.Context, name string, delta int) (Budget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	budget := s.budgets[name]
	budget.Name = name
	budget.Spent += delta
	budget.UpdatedAt = time.Now().UTC()
	s.budgets[name] = budget
	return budget, nil
}

func (s *memStore) Refill(_ context.Context, name string, quota int) (Budget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	budget := s.budgets[name]
	budget.Name = name
	budget.Quota = quota
	budget.Spent = 0
	budget.RefilledAt = time.Now().UTC()
	budget.UpdatedAt = budget.RefilledAt
	s.budgets[name] = budget
	return budget, nil
}

type pgStore struct{ pool *pgxpool.Pool }

func (s *pgStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS trust_budgets (
  name TEXT PRIMARY KEY,
  quota INTEGER NOT NULL DEFAULT 0,
  spent INTEGER NOT NULL DEFAULT 0,
  unlimited BOOLEAN NOT NULL DEFAULT false,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  refilled_at TIMESTAMPTZ
);`)
	return err
}

func (s *pgStore) List(ctx context.Context) ([]Budget, error) {
	rows, err := s.pool.Query(ctx, `SELECT name, quota, spent, unlimited, updated_at, COALESCE(refilled_at, TIMESTAMPTZ 'epoch') FROM trust_budgets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Budget{}
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.Name, &b.Quota, &b.Spent, &b.Unlimited, &b.UpdatedAt, &b.RefilledAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *pgStore) Get(ctx context.Context, name string) (Budget, bool, error) {
	var b Budget
	err := s.pool.QueryRow(ctx, `SELECT name, quota, spent, unlimited, updated_at, COALESCE(refilled_at, TIMESTAMPTZ 'epoch') FROM trust_budgets WHERE name=$1`, name).Scan(&b.Name, &b.Quota, &b.Spent, &b.Unlimited, &b.UpdatedAt, &b.RefilledAt)
	if err != nil {
		return Budget{}, false, nil
	}
	return b, true, nil
}

func (s *pgStore) Upsert(ctx context.Context, b Budget) (Budget, error) {
	err := s.pool.QueryRow(ctx, `
INSERT INTO trust_budgets(name, quota, spent, unlimited, updated_at, refilled_at)
VALUES ($1,$2,$3,$4,NOW(),$5)
ON CONFLICT (name) DO UPDATE SET quota=EXCLUDED.quota, spent=EXCLUDED.spent, unlimited=EXCLUDED.unlimited, updated_at=NOW(), refilled_at=EXCLUDED.refilled_at
RETURNING name, quota, spent, unlimited, updated_at, COALESCE(refilled_at, TIMESTAMPTZ 'epoch')`, b.Name, b.Quota, b.Spent, b.Unlimited, b.RefilledAt).Scan(&b.Name, &b.Quota, &b.Spent, &b.Unlimited, &b.UpdatedAt, &b.RefilledAt)
	return b, err
}

func (s *pgStore) Spend(ctx context.Context, name string, delta int) (Budget, error) {
	_, _ = s.pool.Exec(ctx, `INSERT INTO trust_budgets(name, quota, spent, unlimited, updated_at) VALUES ($1,0,0,false,NOW()) ON CONFLICT (name) DO NOTHING`, name)
	var b Budget
	err := s.pool.QueryRow(ctx, `UPDATE trust_budgets SET spent = spent + $2, updated_at = NOW() WHERE name=$1 RETURNING name, quota, spent, unlimited, updated_at, COALESCE(refilled_at, TIMESTAMPTZ 'epoch')`, name, delta).Scan(&b.Name, &b.Quota, &b.Spent, &b.Unlimited, &b.UpdatedAt, &b.RefilledAt)
	return b, err
}

func (s *pgStore) Refill(ctx context.Context, name string, quota int) (Budget, error) {
	var b Budget
	err := s.pool.QueryRow(ctx, `
INSERT INTO trust_budgets(name, quota, spent, unlimited, updated_at, refilled_at)
VALUES ($1,$2,0,false,NOW(),NOW())
ON CONFLICT (name) DO UPDATE SET quota=$2, spent=0, unlimited=false, updated_at=NOW(), refilled_at=NOW()
RETURNING name, quota, spent, unlimited, updated_at, COALESCE(refilled_at, TIMESTAMPTZ 'epoch')`, name, quota).Scan(&b.Name, &b.Quota, &b.Spent, &b.Unlimited, &b.UpdatedAt, &b.RefilledAt)
	return b, err
}
