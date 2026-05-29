package constitution

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Version struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy int64     `json:"created_by"`
}

type Store interface {
	Init(ctx context.Context) error
	List(ctx context.Context) ([]Version, error)
	Create(ctx context.Context, body string, createdBy int64) (Version, error)
	Activate(ctx context.Context, id string) (Version, error)
	GetActive(ctx context.Context) (Version, bool, error)
}

func NewStore(pool *pgxpool.Pool) Store {
	if pool == nil {
		return &memStore{}
	}
	return &pgStore{pool: pool}
}

type memStore struct {
	mu       sync.RWMutex
	versions []Version
}

func (s *memStore) Init(context.Context) error { return nil }
func (s *memStore) List(context.Context) ([]Version, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Version(nil), s.versions...), nil
}
func (s *memStore) Create(_ context.Context, body string, createdBy int64) (Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := Version{ID: uuid.NewString(), Version: len(s.versions) + 1, Body: body, CreatedBy: createdBy, CreatedAt: time.Now().UTC()}
	s.versions = append(s.versions, v)
	return v, nil
}
func (s *memStore) Activate(_ context.Context, id string) (Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Version
	for i := range s.versions {
		s.versions[i].Active = s.versions[i].ID == id
		if s.versions[i].Active {
			out = s.versions[i]
		}
	}
	return out, nil
}
func (s *memStore) GetActive(_ context.Context) (Version, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.versions {
		if v.Active {
			return v, true, nil
		}
	}
	return Version{}, false, nil
}

type pgStore struct{ pool *pgxpool.Pool }

func (s *pgStore) Init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS constitutions (
  id TEXT PRIMARY KEY,
  version INTEGER NOT NULL,
  body TEXT NOT NULL,
  active BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by BIGINT NOT NULL DEFAULT 0
);`)
	return err
}
func (s *pgStore) List(ctx context.Context) ([]Version, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, version, body, active, created_at, created_by FROM constitutions ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.Version, &v.Body, &v.Active, &v.CreatedAt, &v.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *pgStore) Create(ctx context.Context, body string, createdBy int64) (Version, error) {
	var version int
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM constitutions`).Scan(&version)
	v := Version{ID: uuid.NewString(), Version: version, Body: body, CreatedBy: createdBy}
	err := s.pool.QueryRow(ctx, `INSERT INTO constitutions(id, version, body, active, created_at, created_by) VALUES ($1,$2,$3,false,NOW(),$4) RETURNING created_at`, v.ID, v.Version, v.Body, createdBy).Scan(&v.CreatedAt)
	return v, err
}
func (s *pgStore) Activate(ctx context.Context, id string) (Version, error) {
	_, err := s.pool.Exec(ctx, `UPDATE constitutions SET active = (id = $1)`, id)
	if err != nil {
		return Version{}, err
	}
	var v Version
	err = s.pool.QueryRow(ctx, `SELECT id, version, body, active, created_at, created_by FROM constitutions WHERE id=$1`, id).Scan(&v.ID, &v.Version, &v.Body, &v.Active, &v.CreatedAt, &v.CreatedBy)
	return v, err
}
func (s *pgStore) GetActive(ctx context.Context) (Version, bool, error) {
	var v Version
	err := s.pool.QueryRow(ctx, `SELECT id, version, body, active, created_at, created_by FROM constitutions WHERE active = true ORDER BY version DESC LIMIT 1`).Scan(&v.ID, &v.Version, &v.Body, &v.Active, &v.CreatedAt, &v.CreatedBy)
	if err != nil {
		return Version{}, false, nil
	}
	return v, true, nil
}
